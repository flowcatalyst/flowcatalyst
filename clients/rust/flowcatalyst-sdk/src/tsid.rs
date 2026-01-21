//! TSID (Time-Sorted ID) generation
//!
//! Generates Time-Sorted IDs in Crockford Base32 format.
//! TSIDs are 13-character, lexicographically sortable, URL-safe identifiers.
//!
//! ## Example
//!
//! ```rust
//! use flowcatalyst_sdk::tsid::TsidGenerator;
//!
//! let generator = TsidGenerator::new();
//!
//! // Generate a new TSID
//! let id = generator.generate();
//! println!("Generated TSID: {}", id); // e.g., "0HZXEQ5Y8JY5Z"
//!
//! // Convert between formats
//! let long_value = TsidGenerator::to_long(&id).unwrap();
//! let string_value = TsidGenerator::to_string(long_value);
//! assert_eq!(id, string_value);
//! ```

use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{SystemTime, UNIX_EPOCH};

/// Crockford Base32 alphabet (excludes I, L, O, U to avoid confusion)
const CROCKFORD_ALPHABET: &[u8; 32] = b"0123456789ABCDEFGHJKMNPQRSTVWXYZ";

/// TSID epoch (2020-01-01 00:00:00 UTC)
const TSID_EPOCH: u64 = 1577836800000;

/// TSID generator
///
/// Generates Time-Sorted IDs with the following structure:
/// - 42 bits: timestamp (milliseconds since TSID epoch)
/// - 22 bits: random/counter component
#[derive(Debug)]
pub struct TsidGenerator {
    counter: AtomicU64,
    node_id: u16,
}

impl TsidGenerator {
    /// Create a new TSID generator with a random node ID
    pub fn new() -> Self {
        Self::with_node(rand_node_id())
    }

    /// Create a new TSID generator with a specific node ID
    pub fn with_node(node_id: u16) -> Self {
        Self {
            counter: AtomicU64::new(rand_counter()),
            node_id: node_id & 0x3FF, // 10-bit node ID
        }
    }

    /// Generate a new TSID as a Crockford Base32 string
    pub fn generate(&self) -> String {
        let value = self.generate_long();
        Self::to_string(value)
    }

    /// Generate a new TSID as a 64-bit integer
    pub fn generate_long(&self) -> u64 {
        let timestamp = current_millis() - TSID_EPOCH;
        let counter = self.counter.fetch_add(1, Ordering::Relaxed) & 0xFFF; // 12-bit counter
        let node = (self.node_id as u64) & 0x3FF; // 10-bit node

        // Structure: timestamp (42 bits) | node (10 bits) | counter (12 bits)
        ((timestamp & 0x3FFFFFFFFFF) << 22) | (node << 12) | counter
    }

    /// Convert a TSID string to a 64-bit integer
    pub fn to_long(tsid: &str) -> Option<u64> {
        if tsid.len() != 13 {
            return None;
        }

        let mut value: u64 = 0;
        for c in tsid.chars() {
            let digit = decode_char(c)?;
            value = value
                .checked_mul(32)?
                .checked_add(digit as u64)?;
        }
        Some(value)
    }

    /// Convert a 64-bit integer to a TSID string
    pub fn to_string(value: u64) -> String {
        let mut result = [b'0'; 13];
        let mut v = value;

        for i in (0..13).rev() {
            result[i] = CROCKFORD_ALPHABET[(v & 0x1F) as usize];
            v >>= 5;
        }

        String::from_utf8(result.to_vec()).expect("valid UTF-8")
    }

    /// Extract the timestamp from a TSID
    pub fn timestamp(tsid: &str) -> Option<u64> {
        let value = Self::to_long(tsid)?;
        Some((value >> 22) + TSID_EPOCH)
    }
}

impl Default for TsidGenerator {
    fn default() -> Self {
        Self::new()
    }
}

/// Decode a Crockford Base32 character to its numeric value
fn decode_char(c: char) -> Option<u8> {
    match c.to_ascii_uppercase() {
        '0' | 'O' => Some(0),
        '1' | 'I' | 'L' => Some(1),
        '2' => Some(2),
        '3' => Some(3),
        '4' => Some(4),
        '5' => Some(5),
        '6' => Some(6),
        '7' => Some(7),
        '8' => Some(8),
        '9' => Some(9),
        'A' => Some(10),
        'B' => Some(11),
        'C' => Some(12),
        'D' => Some(13),
        'E' => Some(14),
        'F' => Some(15),
        'G' => Some(16),
        'H' => Some(17),
        'J' => Some(18),
        'K' => Some(19),
        'M' => Some(20),
        'N' => Some(21),
        'P' => Some(22),
        'Q' => Some(23),
        'R' => Some(24),
        'S' => Some(25),
        'T' => Some(26),
        'V' => Some(27),
        'W' => Some(28),
        'X' => Some(29),
        'Y' => Some(30),
        'Z' => Some(31),
        _ => None,
    }
}

/// Get current time in milliseconds since Unix epoch
fn current_millis() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("Time went backwards")
        .as_millis() as u64
}

/// Generate a random node ID
fn rand_node_id() -> u16 {
    use std::collections::hash_map::RandomState;
    use std::hash::{BuildHasher, Hasher};

    let state = RandomState::new();
    let mut hasher = state.build_hasher();
    hasher.write_u64(current_millis());
    (hasher.finish() & 0x3FF) as u16
}

/// Generate a random counter value
fn rand_counter() -> u64 {
    use std::collections::hash_map::RandomState;
    use std::hash::{BuildHasher, Hasher};

    let state = RandomState::new();
    let mut hasher = state.build_hasher();
    hasher.write_u64(current_millis());
    hasher.finish() & 0xFFF
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_generate_tsid() {
        let generator = TsidGenerator::new();
        let tsid = generator.generate();

        assert_eq!(tsid.len(), 13);
        assert!(tsid.chars().all(|c| CROCKFORD_ALPHABET.contains(&(c as u8))));
    }

    #[test]
    fn test_roundtrip_conversion() {
        let generator = TsidGenerator::new();
        let tsid = generator.generate();

        let long_value = TsidGenerator::to_long(&tsid).unwrap();
        let back_to_string = TsidGenerator::to_string(long_value);

        assert_eq!(tsid, back_to_string);
    }

    #[test]
    fn test_uniqueness() {
        let generator = TsidGenerator::new();
        let mut ids: Vec<String> = (0..1000).map(|_| generator.generate()).collect();

        ids.sort();
        ids.dedup();

        assert_eq!(ids.len(), 1000, "All TSIDs should be unique");
    }

    #[test]
    fn test_sortability() {
        let generator = TsidGenerator::new();
        let mut ids: Vec<String> = Vec::new();

        for _ in 0..100 {
            ids.push(generator.generate());
            std::thread::sleep(std::time::Duration::from_millis(1));
        }

        let mut sorted = ids.clone();
        sorted.sort();

        // IDs generated later should sort after earlier ones
        assert_eq!(ids, sorted, "TSIDs should be chronologically sortable");
    }
}
