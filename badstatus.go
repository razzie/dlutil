package dlutil

import (
	"fmt"
	"net/http"
)

type BadStatusError struct {
	StatusCode int
}

func (e BadStatusError) Error() string {
	return fmt.Sprintf("%d %s", e.StatusCode, http.StatusText(e.StatusCode))
}

func BadStatus(statusCode int) *BadStatusError {
	return &BadStatusError{StatusCode: statusCode}
}
