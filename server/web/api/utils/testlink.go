package utils

import (
	"errors"
	"net/url"
)

func TestLink(link string, auth bool) error {
	link, err := url.QueryUnescape(link)
	if err != nil {
		return err
	}

	ur, err := url.Parse(link)
	if err != nil {
		return err
	}

	if ur.Scheme == "magnet" || ur.Scheme == "torrs" || ur.Scheme == "" {
		return nil
	}

	if !auth {
		return errors.New("auth required")
	}

	return nil
}
