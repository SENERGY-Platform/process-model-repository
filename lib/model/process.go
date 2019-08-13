package model

import "errors"

type Process struct {
	Id string
}

func (process *Process) Validate() error {
	if process.Id == "" {
		return errors.New("missing id")
	}
	return nil
}
