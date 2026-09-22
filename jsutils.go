//go:build js && wasm

package wasmsqlite

import (
	"database/sql"
	"fmt"
	"syscall/js"
	"time"
)

// callAsync calls a JavaScript async function and waits for the result
func callAsync(method js.Value, args ...any) (js.Value, error) {
	// Call the method
	promise := method.Invoke(args...)
	if promise.IsUndefined() {
		return js.Undefined(), fmt.Errorf("method did not return a promise")
	}

	// Wait for the promise to resolve
	done := make(chan struct {
		result js.Value
		err    error
	}, 1)

	// Handle promise resolution
	then := js.FuncOf(func(this js.Value, args []js.Value) any {
		defer func() {
			if r := recover(); r != nil {
				done <- struct {
					result js.Value
					err    error
				}{js.Undefined(), fmt.Errorf("promise then handler panicked: %v", r)}
			}
		}()

		var result js.Value
		if len(args) > 0 {
			result = args[0]
		} else {
			result = js.Undefined()
		}

		// Check if result indicates error
		if !result.IsUndefined() && !result.Get("ok").IsUndefined() {
			if !result.Get("ok").Bool() {
				errorMsg := "unknown error"
				if !result.Get("error").IsUndefined() {
					errorMsg = result.Get("error").String()
				}
				done <- struct {
					result js.Value
					err    error
				}{js.Undefined(), fmt.Errorf("%s", errorMsg)}
				return nil
			}
		}

		done <- struct {
			result js.Value
			err    error
		}{result, nil}
		return nil
	})
	defer then.Release()

	// Handle promise rejection
	catch := js.FuncOf(func(this js.Value, args []js.Value) any {
		defer func() {
			if r := recover(); r != nil {
				done <- struct {
					result js.Value
					err    error
				}{js.Undefined(), fmt.Errorf("promise catch handler panicked: %v", r)}
			}
		}()

		errorMsg := "unknown error"
		if len(args) > 0 {
			error := args[0]
			// Try to extract more details from the error
			if !error.IsUndefined() {
				if !error.Get("message").IsUndefined() {
					errorMsg = error.Get("message").String()
				} else if !error.Get("toString").IsUndefined() {
					errorMsg = error.Call("toString").String()
				} else {
					errorMsg = error.String()
				}
			}
		}

		done <- struct {
			result js.Value
			err    error
		}{js.Undefined(), fmt.Errorf("%s", errorMsg)}
		return nil
	})
	defer catch.Release()

	// Attach handlers
	promise.Call("then", then).Call("catch", catch)

	// Wait for completion
	result := <-done
	return result.result, result.err
}

// toJSValue safely converts a Go value to a JavaScript value, handling nil and special cases
func toJSValue(v any) js.Value {
	if v == nil {
		return js.Null()
	}

	// Handle common database types
	switch val := v.(type) {
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64,
		float32, float64, string:
		// These types are handled directly by js.ValueOf
		return js.ValueOf(val)
	case []byte:
		// Convert byte slice to Uint8Array
		if len(val) == 0 {
			return js.Null()
		}
		uint8Array := js.Global().Get("Uint8Array").New(len(val))
		js.CopyBytesToJS(uint8Array, val)
		return uint8Array
	case time.Time:
		// Convert time to ISO string
		if val.IsZero() {
			return js.Null()
		}
		return js.ValueOf(val.Format(time.RFC3339))
	case *time.Time:
		// Handle pointer to time
		if val == nil || val.IsZero() {
			return js.Null()
		}
		return js.ValueOf(val.Format(time.RFC3339))
	case sql.NullString:
		if val.Valid {
			return js.ValueOf(val.String)
		}
		return js.Null()
	case sql.NullBool:
		if val.Valid {
			return js.ValueOf(val.Bool)
		}
		return js.Null()
	case sql.NullInt64:
		if val.Valid {
			return js.ValueOf(val.Int64)
		}
		return js.Null()
	case sql.NullFloat64:
		if val.Valid {
			return js.ValueOf(val.Float64)
		}
		return js.Null()
	case sql.NullTime:
		if val.Valid {
			return js.ValueOf(val.Time.Format(time.RFC3339))
		}
		return js.Null()
	default:
		// For any other type, try to convert it to a string
		// This prevents panics but may not be ideal for all types
		// Use fmt.Sprint as a fallback
		return js.ValueOf(fmt.Sprintf("%v", val))
	}
}
