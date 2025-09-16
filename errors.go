package gostc

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"
)

// Common error variables for consistent error checking
var (
	ErrPathTraversal    = errors.New("path traversal attempt detected")
	ErrInvalidPath      = errors.New("invalid path")
	ErrFileTooLarge     = errors.New("file size exceeds maximum limit")
	ErrRequestTooLarge  = errors.New("request body too large")
	ErrCacheCorrupted   = errors.New("cache entry corrupted")
	ErrCompressionFailed = errors.New("compression failed")
	ErrInvalidConfig    = errors.New("invalid configuration")
	ErrServerShutdown   = errors.New("server is shutting down")
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
	ErrInvalidCSRFToken = errors.New("invalid CSRF token")
	ErrTimeout          = errors.New("operation timed out")
)

// ErrorType represents the category of error
type ErrorType int

const (
	ErrorTypeValidation ErrorType = iota
	ErrorTypeNotFound
	ErrorTypePermission
	ErrorTypeRateLimit
	ErrorTypeServerError
	ErrorTypeTimeout
	ErrorTypeConfiguration
	ErrorTypeSecurity
)

// ServerError represents a detailed error with context
type ServerError struct {
	Type       ErrorType
	Op         string    // Operation that failed
	Path       string    // File path if applicable
	Err        error     // Underlying error
	Message    string    // User-friendly message
	StatusCode int       // HTTP status code
	Timestamp  time.Time // When the error occurred
	RequestID  string    // Request ID for tracing
	Stack      string    // Stack trace for debugging
}

// Error implements the error interface
func (e *ServerError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Op, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Op, e.Message)
}

// Unwrap returns the underlying error
func (e *ServerError) Unwrap() error {
	return e.Err
}

// Is implements errors.Is interface
func (e *ServerError) Is(target error) bool {
	if target == nil {
		return false
	}

	// Check if target is a ServerError with same type
	if se, ok := target.(*ServerError); ok {
		return e.Type == se.Type
	}

	// Check underlying error
	return errors.Is(e.Err, target)
}

// HTTPStatus returns the appropriate HTTP status code for the error
func (e *ServerError) HTTPStatus() int {
	if e.StatusCode != 0 {
		return e.StatusCode
	}

	switch e.Type {
	case ErrorTypeValidation:
		return http.StatusBadRequest
	case ErrorTypeNotFound:
		return http.StatusNotFound
	case ErrorTypePermission:
		return http.StatusForbidden
	case ErrorTypeRateLimit:
		return http.StatusTooManyRequests
	case ErrorTypeTimeout:
		return http.StatusRequestTimeout
	case ErrorTypeConfiguration:
		return http.StatusInternalServerError
	case ErrorTypeSecurity:
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

// UserMessage returns a safe message for the user
func (e *ServerError) UserMessage() string {
	if e.Message != "" {
		return e.Message
	}

	switch e.Type {
	case ErrorTypeValidation:
		return "Invalid request"
	case ErrorTypeNotFound:
		return "Resource not found"
	case ErrorTypePermission:
		return "Permission denied"
	case ErrorTypeRateLimit:
		return "Too many requests. Please try again later"
	case ErrorTypeTimeout:
		return "Request timed out"
	case ErrorTypeSecurity:
		return "Security violation detected"
	default:
		return "An internal error occurred"
	}
}

// NewServerError creates a new ServerError with context
func NewServerError(errType ErrorType, op string, err error) *ServerError {
	se := &ServerError{
		Type:      errType,
		Op:        op,
		Err:       err,
		Timestamp: time.Now(),
		Stack:     getStackTrace(),
	}

	// Set default status code based on type
	se.StatusCode = se.HTTPStatus()

	return se
}

// WithPath adds path context to the error
func (e *ServerError) WithPath(path string) *ServerError {
	e.Path = path
	return e
}

// WithMessage adds a user-friendly message
func (e *ServerError) WithMessage(msg string) *ServerError {
	e.Message = msg
	return e
}

// WithRequestID adds request ID for tracing
func (e *ServerError) WithRequestID(id string) *ServerError {
	e.RequestID = id
	return e
}

// WithStatusCode overrides the default status code
func (e *ServerError) WithStatusCode(code int) *ServerError {
	e.StatusCode = code
	return e
}

// getStackTrace captures the current stack trace
func getStackTrace() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// ErrorHandler handles errors consistently across the application
type ErrorHandler struct {
	logger         *ErrorLogger
	debug          bool
	includeStack   bool
}

// NewErrorHandler creates a new error handler
func NewErrorHandler(debug bool) *ErrorHandler {
	return &ErrorHandler{
		logger:       NewErrorLogger(),
		debug:        debug,
		includeStack: debug,
	}
}

// HandleError processes an error and sends appropriate response
func (eh *ErrorHandler) HandleError(w http.ResponseWriter, r *http.Request, err error) {
	// Extract or create ServerError
	var serverErr *ServerError
	if !errors.As(err, &serverErr) {
		serverErr = NewServerError(ErrorTypeServerError, "server.request", err)
	}

	// Add request context
	if reqID := r.Context().Value("request-id"); reqID != nil {
		serverErr.WithRequestID(fmt.Sprintf("%v", reqID))
	}

	// Log the error
	eh.logger.LogError(serverErr, r)

	// Send response
	eh.sendErrorResponse(w, r, serverErr)
}

// sendErrorResponse sends an appropriate HTTP error response
func (eh *ErrorHandler) sendErrorResponse(w http.ResponseWriter, r *http.Request, err *ServerError) {
	// Set security headers
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")

	// Set status code
	statusCode := err.HTTPStatus()

	// Add retry-after header for rate limit errors
	if err.Type == ErrorTypeRateLimit {
		w.Header().Set("Retry-After", "60")
	}

	// Prepare response body
	message := err.UserMessage()

	// In debug mode, include more details
	if eh.debug {
		message = fmt.Sprintf("%s\nOperation: %s\nError: %v",
			message, err.Op, err.Err)

		if eh.includeStack && err.Stack != "" {
			message += "\n\nStack trace:\n" + err.Stack
		}
	}

	// Send response
	http.Error(w, message, statusCode)
}

// ErrorLogger handles structured error logging
type ErrorLogger struct {
	mu     sync.Mutex
	errors []LoggedError
}

// LoggedError represents an error with metadata
type LoggedError struct {
	Error     *ServerError
	Timestamp time.Time
	Method    string
	Path      string
	ClientIP  string
	UserAgent string
}

// NewErrorLogger creates a new error logger
func NewErrorLogger() *ErrorLogger {
	return &ErrorLogger{
		errors: make([]LoggedError, 0),
	}
}

// LogError logs an error with context
func (el *ErrorLogger) LogError(err *ServerError, r *http.Request) {
	el.mu.Lock()
	defer el.mu.Unlock()

	loggedErr := LoggedError{
		Error:     err,
		Timestamp: time.Now(),
		Method:    r.Method,
		Path:      r.URL.Path,
		ClientIP:  getClientIP(r),
		UserAgent: r.UserAgent(),
	}

	el.errors = append(el.errors, loggedErr)

	// Log to stdout/stderr
	if err.Type == ErrorTypeServerError || err.Type == ErrorTypeConfiguration {
		// Critical errors to stderr
		fmt.Fprintf(stderr, "[ERROR] %s %s %s: %v (Request ID: %s)\n",
			loggedErr.Timestamp.Format(time.RFC3339),
			loggedErr.Method,
			loggedErr.Path,
			err.Error(),
			err.RequestID,
		)
	} else {
		// Info/warning to stdout
		fmt.Printf("[WARN] %s %s %s: %v (Request ID: %s)\n",
			loggedErr.Timestamp.Format(time.RFC3339),
			loggedErr.Method,
			loggedErr.Path,
			err.Error(),
			err.RequestID,
		)
	}
}

// GetRecentErrors returns recent errors for monitoring
func (el *ErrorLogger) GetRecentErrors(limit int) []LoggedError {
	el.mu.Lock()
	defer el.mu.Unlock()

	if len(el.errors) <= limit {
		return append([]LoggedError{}, el.errors...)
	}

	return append([]LoggedError{}, el.errors[len(el.errors)-limit:]...)
}

// RetryableError wraps an error that can be retried
type RetryableError struct {
	Err        error
	Retries    int
	MaxRetries int
	Backoff    time.Duration
}

// Error implements error interface
func (e *RetryableError) Error() string {
	return fmt.Sprintf("operation failed after %d retries: %v", e.Retries, e.Err)
}

// CanRetry checks if the operation can be retried
func (e *RetryableError) CanRetry() bool {
	return e.Retries < e.MaxRetries
}

// NextBackoff calculates the next backoff duration
func (e *RetryableError) NextBackoff() time.Duration {
	// Exponential backoff with jitter
	backoff := e.Backoff * time.Duration(1<<e.Retries)
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}
	return backoff
}

// RetryOperation executes an operation with retry logic
func RetryOperation(op func() error, maxRetries int) error {
	var lastErr error
	backoff := 100 * time.Millisecond

	for i := 0; i <= maxRetries; i++ {
		if err := op(); err != nil {
			lastErr = err

			// Check if error is retryable
			if !isRetryableError(err) {
				return err
			}

			if i < maxRetries {
				time.Sleep(backoff)
				backoff *= 2
				if backoff > 5*time.Second {
					backoff = 5 * time.Second
				}
			}
		} else {
			return nil
		}
	}

	return &RetryableError{
		Err:        lastErr,
		Retries:    maxRetries,
		MaxRetries: maxRetries,
		Backoff:    backoff,
	}
}

// isRetryableError determines if an error is retryable
func isRetryableError(err error) bool {
	// Don't retry validation errors
	var serverErr *ServerError
	if errors.As(err, &serverErr) {
		switch serverErr.Type {
		case ErrorTypeValidation, ErrorTypePermission, ErrorTypeSecurity:
			return false
		}
	}

	// Retry timeout and temporary errors
	if errors.Is(err, ErrTimeout) {
		return true
	}

	// Check for temporary network errors
	type temporary interface {
		Temporary() bool
	}
	if te, ok := err.(temporary); ok {
		return te.Temporary()
	}

	return false
}

var stderr = os.Stderr

// Error recovery functions
func SafeClose(closer io.Closer) {
	if closer != nil {
		if err := closer.Close(); err != nil {
			fmt.Fprintf(stderr, "Error closing resource: %v\n", err)
		}
	}
}

// PanicHandler recovers from panics and converts them to errors
func PanicHandler(op string) (err error) {
	if r := recover(); r != nil {
		err = NewServerError(ErrorTypeServerError, op, fmt.Errorf("panic: %v", r))
	}
	return
}