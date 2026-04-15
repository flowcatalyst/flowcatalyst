//! Use Case Result Type
//!
//! Result type for use case execution. In the FlowCatalyst pattern, success
//! is typically created through `UnitOfWork::commit()` to ensure domain events
//! are always emitted. The SDK makes `success()` public so consumers can
//! implement custom `UnitOfWork` backends.

use super::error::UseCaseError;

/// Result type for use case execution.
///
/// # Usage
///
/// ```ignore
/// // Return failure for validation/business rule violations
/// if !is_valid {
///     return UseCaseResult::failure(UseCaseError::validation("INVALID", "Invalid input"));
/// }
///
/// // Return success through UnitOfWork.commit()
/// unit_of_work.commit(aggregate, event, command).await
/// ```
pub enum UseCaseResult<T> {
    Success(T),
    Failure(UseCaseError),
}

impl<T> UseCaseResult<T> {
    /// Create a failure result.
    pub fn failure(error: UseCaseError) -> Self {
        UseCaseResult::Failure(error)
    }

    /// Create a success result.
    ///
    /// In the standard pattern, success is created by `UnitOfWork::commit()`.
    /// This is public in the SDK to allow custom `UnitOfWork` implementations.
    pub fn success(value: T) -> Self {
        UseCaseResult::Success(value)
    }

    pub fn is_success(&self) -> bool {
        matches!(self, UseCaseResult::Success(_))
    }

    pub fn is_failure(&self) -> bool {
        matches!(self, UseCaseResult::Failure(_))
    }

    pub fn unwrap(self) -> T {
        match self {
            UseCaseResult::Success(v) => v,
            UseCaseResult::Failure(e) => panic!("Called unwrap on a Failure: {}", e),
        }
    }

    pub fn unwrap_or(self, default: T) -> T {
        match self {
            UseCaseResult::Success(v) => v,
            UseCaseResult::Failure(_) => default,
        }
    }

    pub fn unwrap_or_else<F>(self, f: F) -> T
    where
        F: FnOnce(UseCaseError) -> T,
    {
        match self {
            UseCaseResult::Success(v) => v,
            UseCaseResult::Failure(e) => f(e),
        }
    }

    pub fn unwrap_err(self) -> UseCaseError {
        match self {
            UseCaseResult::Success(_) => panic!("Called unwrap_err on a Success"),
            UseCaseResult::Failure(e) => e,
        }
    }

    pub fn as_ref(&self) -> UseCaseResult<&T> {
        match self {
            UseCaseResult::Success(v) => UseCaseResult::Success(v),
            UseCaseResult::Failure(e) => UseCaseResult::Failure(e.clone()),
        }
    }

    pub fn map<U, F>(self, f: F) -> UseCaseResult<U>
    where
        F: FnOnce(T) -> U,
    {
        match self {
            UseCaseResult::Success(v) => UseCaseResult::Success(f(v)),
            UseCaseResult::Failure(e) => UseCaseResult::Failure(e),
        }
    }

    pub fn map_err<F>(self, f: F) -> UseCaseResult<T>
    where
        F: FnOnce(UseCaseError) -> UseCaseError,
    {
        match self {
            UseCaseResult::Success(v) => UseCaseResult::Success(v),
            UseCaseResult::Failure(e) => UseCaseResult::Failure(f(e)),
        }
    }

    pub fn into_result(self) -> Result<T, UseCaseError> {
        match self {
            UseCaseResult::Success(v) => Ok(v),
            UseCaseResult::Failure(e) => Err(e),
        }
    }
}

impl<T> From<UseCaseResult<T>> for Result<T, UseCaseError> {
    fn from(result: UseCaseResult<T>) -> Self {
        result.into_result()
    }
}

impl<T: std::fmt::Debug> std::fmt::Debug for UseCaseResult<T> {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            UseCaseResult::Success(v) => f.debug_tuple("Success").field(v).finish(),
            UseCaseResult::Failure(e) => f.debug_tuple("Failure").field(e).finish(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn success_creation() {
        let result: UseCaseResult<i32> = UseCaseResult::success(42);
        assert!(result.is_success());
        assert!(!result.is_failure());
    }

    #[test]
    fn failure_creation() {
        let result: UseCaseResult<i32> =
            UseCaseResult::failure(UseCaseError::validation("V", "bad input"));
        assert!(result.is_failure());
        assert!(!result.is_success());
    }

    #[test]
    fn unwrap_success() {
        let result = UseCaseResult::success("hello");
        assert_eq!(result.unwrap(), "hello");
    }

    #[test]
    #[should_panic(expected = "Called unwrap on a Failure")]
    fn unwrap_failure_panics() {
        let result: UseCaseResult<()> =
            UseCaseResult::failure(UseCaseError::validation("V", "fail"));
        result.unwrap();
    }

    #[test]
    fn unwrap_or_returns_value_on_success() {
        let result = UseCaseResult::success(10);
        assert_eq!(result.unwrap_or(0), 10);
    }

    #[test]
    fn unwrap_or_returns_default_on_failure() {
        let result: UseCaseResult<i32> =
            UseCaseResult::failure(UseCaseError::not_found("NF", "gone"));
        assert_eq!(result.unwrap_or(99), 99);
    }

    #[test]
    fn unwrap_or_else_success() {
        let result = UseCaseResult::success(5);
        let val = result.unwrap_or_else(|_| 0);
        assert_eq!(val, 5);
    }

    #[test]
    fn unwrap_or_else_failure() {
        let result: UseCaseResult<String> =
            UseCaseResult::failure(UseCaseError::validation("V", "msg"));
        let val = result.unwrap_or_else(|e| format!("error: {}", e.code()));
        assert_eq!(val, "error: V");
    }

    #[test]
    fn unwrap_err_on_failure() {
        let result: UseCaseResult<()> =
            UseCaseResult::failure(UseCaseError::commit("db error"));
        let err = result.unwrap_err();
        assert_eq!(err.code(), "COMMIT_FAILED");
    }

    #[test]
    #[should_panic(expected = "Called unwrap_err on a Success")]
    fn unwrap_err_on_success_panics() {
        let result = UseCaseResult::success(42);
        result.unwrap_err();
    }

    #[test]
    fn map_transforms_success() {
        let result = UseCaseResult::success(10);
        let mapped = result.map(|v| v * 2);
        assert_eq!(mapped.unwrap(), 20);
    }

    #[test]
    fn map_preserves_failure() {
        let result: UseCaseResult<i32> =
            UseCaseResult::failure(UseCaseError::not_found("NF", "x"));
        let mapped = result.map(|v| v * 2);
        assert!(mapped.is_failure());
        assert_eq!(mapped.unwrap_err().code(), "NF");
    }

    #[test]
    fn map_err_transforms_failure() {
        let result: UseCaseResult<i32> =
            UseCaseResult::failure(UseCaseError::validation("OLD", "old msg"));
        let mapped = result.map_err(|_| UseCaseError::business_rule("NEW", "new msg"));
        assert_eq!(mapped.unwrap_err().code(), "NEW");
    }

    #[test]
    fn map_err_preserves_success() {
        let result = UseCaseResult::success(42);
        let mapped = result.map_err(|_| UseCaseError::commit("should not happen"));
        assert_eq!(mapped.unwrap(), 42);
    }

    #[test]
    fn into_result_success() {
        let result = UseCaseResult::success("ok");
        let std_result: Result<&str, UseCaseError> = result.into_result();
        assert_eq!(std_result.unwrap(), "ok");
    }

    #[test]
    fn into_result_failure() {
        let result: UseCaseResult<()> =
            UseCaseResult::failure(UseCaseError::concurrency("STALE", "retry"));
        let std_result: Result<(), UseCaseError> = result.into_result();
        assert!(std_result.is_err());
        assert_eq!(std_result.unwrap_err().code(), "STALE");
    }

    #[test]
    fn from_impl_conversion() {
        let result = UseCaseResult::success(100);
        let converted: Result<i32, UseCaseError> = result.into();
        assert_eq!(converted.unwrap(), 100);
    }

    #[test]
    fn as_ref_success() {
        let result = UseCaseResult::success(42);
        let r = result.as_ref();
        assert_eq!(*r.unwrap(), 42);
    }

    #[test]
    fn as_ref_failure() {
        let result: UseCaseResult<i32> =
            UseCaseResult::failure(UseCaseError::validation("V", "m"));
        let r = result.as_ref();
        assert!(r.is_failure());
    }

    #[test]
    fn debug_impl() {
        let s = UseCaseResult::success(42);
        let debug_str = format!("{:?}", s);
        assert!(debug_str.contains("Success"));
        assert!(debug_str.contains("42"));

        let f: UseCaseResult<i32> =
            UseCaseResult::failure(UseCaseError::validation("V", "msg"));
        let debug_str = format!("{:?}", f);
        assert!(debug_str.contains("Failure"));
    }
}
