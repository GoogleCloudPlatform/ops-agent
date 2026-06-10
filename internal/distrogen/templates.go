// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const EMPTY_FILE_NAME = ".empty"

//go:embed templates/component/*
var embeddedComponentTemplatesFS embed.FS

//go:embed templates/components/*
var embeddedComponentsTemplatesFS embed.FS

//go:embed templates/distribution/*
var embeddedDistributionTemplatesFS embed.FS

//go:embed templates/make/*
var embeddedMakeTemplatesFS embed.FS

//go:embed templates/project/*
var embeddedProjectTemplatesFS embed.FS

//go:embed templates/scripts/*
var embeddedScriptsTemplatesFS embed.FS

// TemplateFile is the information about a template file
// that will be rendered for a distribution.
type TemplateFile struct {
	Name     string
	FilePath string
	FSPath   string
	Context  any
	FS       fs.FS
	FileMode fs.FileMode
}

func GenerateTemplateSet(outDir string, templateSet TemplateSet) error {
	for _, tmpl := range templateSet {
		if err := tmpl.Render(outDir); err != nil {
			logger.Debug(fmt.Sprintf("failed to render %s", tmpl.Name), "err", err)
			return err
		}
	}
	return nil
}

// outputPath gets the intended destination output path for the rendered template.
func (tf *TemplateFile) outputPath() string {
	tfPath := strings.TrimSuffix(tf.FilePath, filepath.Base(tf.FilePath))
	return tfPath
}

// getTextTemplate retrieves the text tempalte from the template file's
// provided filesystem. This may be the embedded filesystem or another
// set provided by the user.
func (tf *TemplateFile) getTextTemplate() (*template.Template, error) {
	return template.
		New(filepath.Base(tf.FilePath)).
		ParseFS(tf.FS, tf.FilePath)
}

// Render will render the template into a file in the
// requested destination.
func (tf *TemplateFile) Render(outDir string) error {
	tmpl, err := tf.getTextTemplate()
	if err != nil {
		return err
	}
	buf := bytes.Buffer{}
	if err := tmpl.Execute(&buf, tf.Context); err != nil {
		return err
	}
	tmplPath := tf.outputPath()
	if tmplPath != "" {
		// If the template is meant to go in a sub-directory, we need to mkdir -p
		// to make sure we can write the file to the directory it belongs in.
		if err := os.MkdirAll(filepath.Join(outDir, tmplPath), fs.ModePerm); err != nil {
			return err
		}
	}
	generateFilePath := filepath.Join(outDir, tmplPath, tf.Name)
	if err := os.WriteFile(
		generateFilePath,
		buf.Bytes(),
		tf.FileMode,
	); err != nil {
		return err
	}
	// HACK: WriteFile respects `umask` on the system but we don't want to.
	// We truly do want read acces to any group in the files we create.
	return os.Chmod(generateFilePath, tf.FileMode)
}

var (
	ErrInvalidTemplateName = errors.New("invalid template name, must end with .go.tmpl")
	ErrTemplateNotFound    = errors.New("template not found")
)

// TemplateSet is a map of template names to a template file.
type TemplateSet map[string]*TemplateFile

// AddTemplate will add a template to the template set. If a template
// is added with a name that already exists, AddTemplate will overwrite
// the template it has.
func (ts TemplateSet) AddTemplate(path string, templateContext any, dir fs.FS, fileMode fs.FileMode) error {
	name := filepath.Base(path)
	ts[name] = &TemplateFile{
		FilePath: path,
		FSPath:   path,
		Name:     strings.TrimSuffix(name, ".go.tmpl"),
		Context:  templateContext,
		FS:       dir,
		FileMode: fileMode,
	}
	return nil
}

// GetTemplate will retrieve a template from the template set.
func (ts TemplateSet) GetTemplate(name string) (*TemplateFile, error) {
	tf, ok := ts[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrTemplateNotFound, name)
	}
	return tf, nil
}

// RenameExceptionalTemplates will take known names from the TemplateSet
// and replace the rendering name with something else. This is generally
// used for templates that need to be named something different depending
// on the contents of the spec.
func (ts TemplateSet) RenameExceptionalTemplates(spec *DistributionSpec) {
	if file, ok := ts["systemd-unit.service.go.tmpl"]; ok {
		file.Name = spec.BinaryName + ".service"
	}
	if file, ok := ts["conf-file.conf.go.tmpl"]; ok {
		file.Name = spec.BinaryName + ".conf"
	}
}

// GetTemplateSetFromDir will walk an FS for any *.go.tmpl files and
// will collect them into a TemplateSet.
func GetTemplateSetFromDir(dir fs.FS, templateContext any, fileMode fs.FileMode) (TemplateSet, error) {
	templates := TemplateSet{}

	err := fs.WalkDir(dir, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".go.tmpl") {
			return nil
		}
		if filepath.Base(path) == EMPTY_FILE_NAME {
			return nil
		}
		return templates.AddTemplate(path, templateContext, dir, fileMode)
	})

	return templates, err
}

func GetDistributionTemplateSet(templateContext any, fileMode fs.FileMode) (TemplateSet, error) {
	embedFSSub, err := fs.Sub(embeddedDistributionTemplatesFS, filepath.Join("templates", "distribution"))
	if err != nil {
		return nil, err
	}
	return getEmbeddedTemplateSet(templateContext, embedFSSub, fileMode)
}

func GetComponentsTemplateSet(templateContext any, fileMode fs.FileMode) (TemplateSet, error) {
	embedFSSub, err := fs.Sub(embeddedComponentsTemplatesFS, filepath.Join("templates", "components"))
	if err != nil {
		return nil, err
	}
	return getEmbeddedTemplateSet(templateContext, embedFSSub, fileMode)
}

func GetIndividualComponentTemplateSet(templateContext any, fileMode fs.FileMode) (TemplateSet, error) {
	embedFSSub, err := fs.Sub(embeddedComponentTemplatesFS, filepath.Join("templates", "component"))
	if err != nil {
		return nil, err
	}
	return getEmbeddedTemplateSet(templateContext, embedFSSub, fileMode)
}

func GetMakeTemplateSet(templateContext any, fileMode fs.FileMode) (TemplateSet, error) {
	embedFSSub, err := fs.Sub(embeddedMakeTemplatesFS, filepath.Join("templates", "make"))
	if err != nil {
		return nil, err
	}
	return getEmbeddedTemplateSet(templateContext, embedFSSub, fileMode)
}

func GetProjectTemplateSet(templateContext any, fileMode fs.FileMode) (TemplateSet, error) {
	embedFSSub, err := fs.Sub(embeddedProjectTemplatesFS, filepath.Join("templates", "project"))
	if err != nil {
		return nil, err
	}
	return getEmbeddedTemplateSet(templateContext, embedFSSub, fileMode)
}

func GetDistrogenTemplateSet(templateContext any, fileMode fs.FileMode) (TemplateSet, error) {
	embedFSSub, err := fs.Sub(embeddedProjectTemplatesFS, filepath.Join("templates", "project", ".distrogen"))
	if err != nil {
		return nil, err
	}
	return getEmbeddedTemplateSet(templateContext, embedFSSub, fileMode)
}

func GetScriptTemplateSet(templateContext any, fileMode fs.FileMode) (TemplateSet, error) {
	embedFSSub, err := fs.Sub(embeddedScriptsTemplatesFS, filepath.Join("templates", "scripts"))
	if err != nil {
		return nil, err
	}
	return getEmbeddedTemplateSet(templateContext, embedFSSub, fileMode)
}

// GetDistributionTemplateSet will get the template set from the template FS embedded
// into the distrogen binary.
func getEmbeddedTemplateSet(templateContext any, embeddedFs fs.FS, fileMode fs.FileMode) (TemplateSet, error) {
	templates, err := GetTemplateSetFromDir(embeddedFs, templateContext, fileMode)
	if err != nil {
		return nil, err
	}
	return templates, nil
}
