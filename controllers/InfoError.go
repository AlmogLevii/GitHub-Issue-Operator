package controllers

import perror "github.com/pkg/errors"

type InfoError struct {
	Err     error
	Message string
}

func newInfoError(err error, message string) InfoError {
	var Err error
	if err == nil {
		Err = perror.Wrap(err, message)
	} else {
		Err = err
	}
	return InfoError{
		Err:     Err,
		Message: message,
	}
}
