package model

import (
	"errors"
	"go.mongodb.org/mongo-driver/bson"
)

type Process struct {
	Id          string `json:"_id" bson:"_id"`
	Date        int64  `json:"date" bson:"date"`
	Owner       string `json:"owner" bson:"owner"`
	Process     bson.M `json:"process" bson:"process"`
	SvgXml      string `json:"svgXML" bson:"svgXML"`
	Publish     bool   `json:"publish" bson:"publish"`
	PublishDate string `json:"publish_date" bson:"publish_date"`
	Description string `json:"description" bson:"description"`
}

type PublicCommand struct {
	Publish     bool   `json:"publish"`
	Description string `json:"description"`
}

func (process *Process) Validate() error {
	if process.Id == "" {
		return errors.New("missing id")
	}
	definitions, ok := process.Process["definitions"].(map[string]interface{})
	if !ok {
		return errors.New("unable to parse process")
	}
	p, ok := definitions["process"].(map[string]interface{})
	if !ok {
		return errors.New("unable to parse process")
	}

	id, ok := p["_id"]
	if !ok {
		return errors.New("mission process definition id")
	}
	idStr, ok := id.(string)
	if !ok || idStr == "" {
		return errors.New("mission process definition id")
	}

	return nil
}
