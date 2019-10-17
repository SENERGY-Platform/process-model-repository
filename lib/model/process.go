package model

import (
	"errors"
	"fmt"
	"github.com/beevik/etree"
	"log"
	"runtime/debug"
)

type Process struct {
	Id          string `json:"_id" bson:"_id"`
	Date        int64  `json:"date" bson:"date"`
	Owner       string `json:"owner" bson:"owner"`
	BpmnXml     string `json:"bpmn_xml" bson:"bpmn_xml"`
	SvgXml      string `json:"svgXML" bson:"svgXML"`
	Publish     bool   `json:"publish" bson:"publish"`
	PublishDate string `json:"publish_date" bson:"publish_date"`
	Description string `json:"description" bson:"description"`
}

type PublicCommand struct {
	Publish     bool   `json:"publish"`
	Description string `json:"description"`
}

func (process *Process) Validate() (err error) {
	defer func() {
		if r := recover(); r != nil && err == nil {
			log.Printf("%s: %s", r, debug.Stack())
			err = errors.New(fmt.Sprint("Recovered Error: ", r))
		}
	}()
	if process.Id == "" {
		return errors.New("missing id")
	}
	doc := etree.NewDocument()
	err = doc.ReadFromString(process.BpmnXml)
	if err != nil {
		return err
	}
	definition := doc.FindElement("//bpmn:process")
	if process == nil {
		return errors.New("missing process definition")
	}
	id := definition.SelectAttrValue("id", "")
	if id == "" {
		return errors.New("missing process definition id")
	}
	return nil
}
