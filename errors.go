package main

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"unicode"
)

func userError(err error) string {
	if err == nil {
		return "Something went wrong. Please try again."
	}
	message := err.Error()
	if strings.Contains(message, "cannot read catalog") || strings.Contains(message, "cannot read library location") {
		return "Card Atlas could not read its saved collection details. Keep the existing catalog file and check the .log folder for details."
	}
	if strings.Contains(message, "recovery files kept") || strings.Contains(message, "recovery failed") || strings.Contains(message, "destination copy retained") {
		return "The operation could not finish, and some files need recovery. Keep the recovery files and check the .log folder for details before trying again."
	}
	var syntax *json.SyntaxError
	var value *json.UnmarshalTypeError
	var size *http.MaxBytesError
	var network net.Error
	var file *os.PathError
	var link *os.LinkError
	var system *os.SyscallError
	switch {
	case errors.As(err, &size):
		return "This upload is too large. Choose fewer photos and try again."
	case errors.As(err, &syntax), errors.As(err, &value), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		return "The request could not be read. Reload the page and try again."
	case errors.Is(err, syscall.ENOSPC):
		return "There is not enough disk space. Free some space and try again."
	case errors.Is(err, os.ErrPermission):
		return "Card Atlas cannot access this file or folder. Check its permissions, then try again."
	case errors.Is(err, os.ErrNotExist):
		return "A required file or folder is no longer available. Check that the collection folder is connected, then reload."
	case errors.Is(err, os.ErrExist):
		return "A file with the same name already exists. Choose a different name or destination and try again."
	case errors.As(err, &network):
		return "Card Atlas could not start its local connection. Close any other running copy or choose another port, then try again."
	case errors.As(err, &file), errors.As(err, &link), errors.As(err, &system):
		return "Card Atlas could not read or save the files. Check the folder and available disk space, then try again. Technical details are in the .log folder."
	}
	if strings.HasPrefix(message, "invalid request:") || strings.Contains(message, "expected application/json") || strings.Contains(message, "request must contain one JSON object") {
		return "The request could not be read. Reload the page and try again."
	}
	if strings.Contains(message, "duplicate import review") {
		return "The import request was sent more than once. Review the photos again, then retry the import."
	}
	if strings.Contains(message, "import review too large") {
		return "There are too many import choices to process at once. Import fewer photos at a time."
	}
	if strings.HasPrefix(message, "upload interrupted;") {
		return "The upload was interrupted. Check that Card Atlas is still running, then review the photos and try again."
	}
	if strings.HasPrefix(message, "expected up to") {
		return "Choose up to 10,000 supported image files and try again."
	}
	if strings.Contains(message, "symbolic link") || strings.Contains(message, "symbolic-link") {
		return "This operation cannot use a linked folder. Choose the original folder and try again."
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return "Something went wrong. Please try again."
	}
	runes := []rune(message)
	runes[0] = unicode.ToUpper(runes[0])
	message = string(runes)
	if !strings.ContainsAny(message[len(message)-1:], ".!?") {
		message += "."
	}
	return message
}
