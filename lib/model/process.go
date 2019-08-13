package model

import (
	"errors"
	"time"
)

type Process struct {
	Id          string      `json:"_id" bson:"_id"`
	Date        int64       `json:"date" bson:"date"`
	Owner       string      `json:"owner" bson:"owner"`
	Process     interface{} `json:"process" bson:"process"`
	SvgXml      string      `json:"svgXML" bson:"svgXML"`
	Publish     bool        `json:"publish" bson:"publish"`
	PublishDate time.Time   `json:"publish_date" bson:"publish_date"`
	Description string      `json:"description" bson:"description"`
}

type PublicCommand struct {
	Publish     bool   `json:"publish"`
	Description string `json:"description"`
}

func (process *Process) Validate() error {
	if process.Id == "" {
		return errors.New("missing id")
	}
	return nil
}
