// Package plist implements the XML-based property list format used by macOS.
//
// https://developer.apple.com/library/archive/documentation/Cocoa/Conceptual/PropertyLists/PropertyLists.html
package plist

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"strconv"
)

// Object represents a property list object. This can be any value, such as a
// string, boolean, integer, array, or dictionary.
type Object interface {
	encodePlist(*bytes.Buffer) error
}

const header = `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`

// Marshal encodes the provided object into a property list.
func Marshal(obj Object) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	buf.WriteString(header)
	buf.WriteString(`<plist version="1.0">`)
	if err := obj.encodePlist(buf); err != nil {
		return nil, err
	}
	buf.WriteString(`</plist>`)
	return buf.Bytes(), nil
}

type plistString string

// String creates a new property list object representing a string.
func String(s string) Object {
	return plistString(s)
}

func (p plistString) encodePlist(buf *bytes.Buffer) error {
	buf.WriteString("<string>")
	if err := xml.EscapeText(buf, []byte(p)); err != nil {
		return err
	}
	buf.WriteString("</string>")
	return nil
}

type plistData []byte

// Data creates a new property list object representing a data.
func Data(d []byte) Object {
	return plistData(d)
}

func (p plistData) encodePlist(buf *bytes.Buffer) error {
	buf.WriteString("<data>")
	data := base64.StdEncoding.EncodeToString(p)
	if err := xml.EscapeText(buf, []byte(data)); err != nil {
		return err
	}
	buf.WriteString("</data>")
	return nil
}

type plistBool bool

// Bool creates a new property list object representing a boolean, either
// <true/> or <false/>.
func Bool(b bool) Object {
	return plistBool(b)
}

func (p plistBool) encodePlist(buf *bytes.Buffer) error {
	if p {
		buf.WriteString("<true/>")
	} else {
		buf.WriteString("<false/>")
	}
	return nil
}

type plistInt int

// Int creates a new property list object representing an integer.
func Int(i int) Object {
	return plistInt(i)
}

func (p plistInt) encodePlist(buf *bytes.Buffer) error {
	buf.WriteString("<integer>")
	s := strconv.Itoa(int(p))
	if err := xml.EscapeText(buf, []byte(s)); err != nil {
		return err
	}
	buf.WriteString("</integer>")
	return nil
}

type plistArray []Object

// Array creates a new property list object representing an array.
func Array(objs ...Object) Object {
	return plistArray(objs)
}

func (p plistArray) encodePlist(buf *bytes.Buffer) error {
	buf.WriteString("<array>")

	for _, obj := range p {
		if err := obj.encodePlist(buf); err != nil {
			return err
		}
	}
	buf.WriteString("</array>")
	return nil
}

type dictEntry struct {
	key   string
	value Object
}

// Dictionary is an ordered key-value pair list.
type Dictionary struct {
	entries []dictEntry
}

func (d *Dictionary) Add(key string, value Object) *Dictionary {
	d.entries = append(d.entries, dictEntry{key: key, value: value})
	return d
}

// Dict creates a new property list object representing a dictionary. Use Add to
// append key-value pairs to the dictionary in order.
func Dict() *Dictionary {
	return &Dictionary{}
}

func (d *Dictionary) encodePlist(buf *bytes.Buffer) error {
	buf.WriteString("<dict>")
	for _, entry := range d.entries {
		buf.WriteString("<key>")
		if err := xml.EscapeText(buf, []byte(entry.key)); err != nil {
			return err
		}
		buf.WriteString("</key>")
		if err := entry.value.encodePlist(buf); err != nil {
			return err
		}
	}
	buf.WriteString("</dict>")
	return nil
}
