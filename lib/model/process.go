package model

import (
	"errors"
	"fmt"
	"github.com/beevik/etree"
	"log"
	"runtime/debug"
)

type ListOptions struct {
	Ids        []string //filter; ignores limit/offset if Ids != nil; ignored if Ids == nil; Ids == []string{} will return an empty list;
	Search     string
	Limit      int64      //default 100, will be ignored if 'ids' is set (Ids != nil)
	Offset     int64      //default 0, will be ignored if 'ids' is set (Ids != nil)
	SortBy     string     //default name.asc
	Permission AuthAction //defaults to read
}

type Process struct {
	Id              string `json:"_id" bson:"_id"`
	Name            string `json:"name" bson:"name"`
	Date            int64  `json:"date" bson:"date"`
	Owner           string `json:"owner" bson:"owner"`
	BpmnXml         string `json:"bpmn_xml" bson:"bpmn_xml"`
	SvgXml          string `json:"svgXML" bson:"svgXML"`
	Publish         bool   `json:"publish" bson:"publish"`
	PublishDate     string `json:"publish_date" bson:"publish_date"`
	Description     string `json:"description" bson:"description"`
	LastUpdatedUnix int64  `json:"last_updated_unix" bson:"last_updated_unix"`
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
	if definition == nil {
		return errors.New("missing process definition")
	}
	id := definition.SelectAttrValue("id", "")
	if id == "" {
		return errors.New("missing process definition id")
	}
	return nil
}
