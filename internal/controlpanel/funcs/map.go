package funcs

import (
	"html/template"
	"reflect"
)

func Map() template.FuncMap {
	return template.FuncMap{
		"hasField": hasField,
	}
}

func hasField(v interface{}, name string) bool {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return false
	}
	return rv.FieldByName(name).IsValid()
}
