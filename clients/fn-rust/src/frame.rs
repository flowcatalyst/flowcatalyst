//! ABI v1 frame codec (plan §5.2/§5.3), used in both directions and for
//! host calls:
//!
//! ```text
//! u32 little-endian  metaLen
//! metaLen bytes      meta   (UTF-8 JSON)
//! remaining bytes    body   (raw)
//! ```
//!
//! Portable: no `cfg(target_arch)` gate, so it is exercised directly by
//! `cargo test` on the host target.

#[derive(Debug, PartialEq, Eq)]
pub enum FrameError {
    TooShort,
    MetaOverrun,
}

impl core::fmt::Display for FrameError {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        match self {
            FrameError::TooShort => write!(f, "frame shorter than the 4-byte length prefix"),
            FrameError::MetaOverrun => write!(f, "frame metaLen exceeds the frame length"),
        }
    }
}

/// Lays out `meta` and `body` as a single ABI v1 frame.
pub fn encode_frame(meta: &[u8], body: &[u8]) -> Vec<u8> {
    let mut buf = Vec::with_capacity(4 + meta.len() + body.len());
    buf.extend_from_slice(&(meta.len() as u32).to_le_bytes());
    buf.extend_from_slice(meta);
    buf.extend_from_slice(body);
    buf
}

/// Splits an ABI v1 frame into its meta and body slices.
pub fn decode_frame(data: &[u8]) -> Result<(&[u8], &[u8]), FrameError> {
    if data.len() < 4 {
        return Err(FrameError::TooShort);
    }
    let meta_len = u32::from_le_bytes([data[0], data[1], data[2], data[3]]) as usize;
    if meta_len > data.len() - 4 {
        return Err(FrameError::MetaOverrun);
    }
    let meta = &data[4..4 + meta_len];
    let body = &data[4 + meta_len..];
    Ok((meta, body))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn round_trip_both_empty() {
        let frame = encode_frame(&[], &[]);
        let (m, b) = decode_frame(&frame).unwrap();
        assert!(m.is_empty());
        assert!(b.is_empty());
    }

    #[test]
    fn round_trip_meta_and_body() {
        let meta = br#"{"status":200}"#;
        let body = br#"{"ok":true}"#;
        let frame = encode_frame(meta, body);
        let (m, b) = decode_frame(&frame).unwrap();
        assert_eq!(m, meta);
        assert_eq!(b, body);
    }

    #[test]
    fn round_trip_binary_body() {
        let body = [0x00u8, 0xff, 0x10, 0x00, 0x00];
        let frame = encode_frame(b"{}", &body);
        let (_, b) = decode_frame(&frame).unwrap();
        assert_eq!(b, body);
    }

    /// Pins the wire layout byte-for-byte: the metaLen prefix must be
    /// little-endian (plan §5.2). A metaLen of 258 (0x0102) must be encoded
    /// as bytes {0x02, 0x01, 0x00, 0x00}. This is the case a
    /// little->big-endian mutation flips.
    #[test]
    fn length_prefix_is_little_endian() {
        let meta = vec![b'x'; 258];
        let frame = encode_frame(&meta, b"body");
        assert_eq!(&frame[0..4], &[0x02, 0x01, 0x00, 0x00]);
    }

    #[test]
    fn decode_too_short() {
        for n in 0..4 {
            let buf = vec![0u8; n];
            assert_eq!(decode_frame(&buf), Err(FrameError::TooShort));
        }
    }

    #[test]
    fn decode_meta_overrun() {
        let mut buf = vec![0u8; 8];
        buf[0] = 100; // metaLen claims 100 bytes, frame only carries 4
        assert_eq!(decode_frame(&buf), Err(FrameError::MetaOverrun));
    }

    #[test]
    fn decode_exact_meta_no_body() {
        let frame = encode_frame(br#"{"a":1}"#, &[]);
        let (m, b) = decode_frame(&frame).unwrap();
        assert_eq!(m, br#"{"a":1}"#);
        assert!(b.is_empty());
    }
}
