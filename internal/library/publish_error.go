package library

import "fmt"

type SavedButUnpublishedError struct {
	Subject string
	Err     error
}

func (e *SavedButUnpublishedError) Error() string {
	return fmt.Sprintf(
		"%s was saved, but the published website could not be updated; run `bookshelf build` to retry: %v",
		e.Subject,
		e.Err,
	)
}

func (e *SavedButUnpublishedError) Unwrap() error {
	return e.Err
}

func savedButUnpublished(subject string, err error) error {
	if err == nil {
		return nil
	}
	return &SavedButUnpublishedError{Subject: subject, Err: err}
}
