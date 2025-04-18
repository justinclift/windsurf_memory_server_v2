// Package docs GENERATED BY SWAG; DO NOT EDIT
package docs

import "github.com/swaggo/swag"

func init() {
	swag.Register(swag.Name, &s{})
}

type s struct{}

func (s *s) ReadDoc() string {
	return `{
    "openapi": "3.0.0",
    "info": {
        "title": "Windsurf Memory Server API",
        "version": "1.0",
        "description": "API for storing and managing versioned memories."
    },
    "paths": {}
}`
}
