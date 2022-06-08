package plan

import (
	"fmt"

	"github.com/dagger/dlsp/file"
	"github.com/dagger/dlsp/internal"
	"github.com/dagger/dlsp/loader"
	"github.com/tliron/kutil/logging"
)

// Plan is a representation of a cue value in a workspace
type Plan struct {
	// Root path of the plan
	rootPath string

	// RootFile path
	rootFilePath string

	// Files loaded
	files map[string]*file.File

	// Plan's kind
	kind Kind

	// Plan's instance
	instance *loader.Instance

	// Cue Value
	v *loader.Value

	// Imported packages
	// We use a map because for performance reason
	// See https://boltandnuts.wordpress.com/2017/11/20/go-slice-vs-maps/
	imports map[string]*loader.Instance

	log logging.Logger
}

// New load a new cue value
func New(root, filePath string) (*Plan, error) {
	log := logging.GetLogger(fmt.Sprintf("plan: %s", filePath))

	k := File
	log.Debugf("Try to load plan as file")
	i, err := loader.File(root, filePath)

	if err != nil {
		log.Debugf("Try to load plan as directory")
		i, err = loader.Dir(root, filePath)
		if err != nil {
			return nil, err
		}

		k = Directory
	}

	log.Debugf("Plan loaded")

	v, err := i.GetValue()
	if err != nil {
		return nil, err
	}

	f, err := file.New(filePath)
	if err != nil {
		return nil, err
	}

	files := make(map[string]*file.File)
	files[filePath] = f

	// Load cue value
	p := &Plan{
		rootPath:     root,
		rootFilePath: filePath,
		files:        files,
		kind:         k,
		instance:     i,
		v:            v,
		log:          log,
		imports:      make(map[string]*loader.Instance),
	}

	if err := p.loadImports(); err != nil {
		return nil, err
	}

	if err := p.instance.LoadDefinitions(); err != nil {
		return nil, err
	}

	return p, nil
}

// loadImports will explore plan's value and list all definitions contained
// in current values and imported packages
func (p *Plan) loadImports() error {
	for _, i := range p.instance.Imports {
		i := loader.NewInstance(i)
		err := i.LoadDefinitions()
		if err != nil {
			return err
		}

		p.imports[i.PkgName] = i
	}

	return nil
}

// GetDefinition return a value following a path
// TODO(TomChv): define path format
// TODO(TomChv): Can be optimized with path, for instance
// - `.#Foo` = definition in current plan
// - `pkg.#Bar` = definition in package pkg
func (p *Plan) GetDefinition(path string, line, char int) (*loader.Value, error) {
	p.log.Debugf("Looking for file: %s", path)
	f, found := p.files[path]
	if !found {
		return nil, fmt.Errorf("file not registered")
	}

	p.log.Debugf("Looking for def in %s at {%d, %d}", path, line, char)
	def, err := f.Defs().Find(line, char)
	if err != nil {
		return nil, err
	}

	p.log.Debugf("Searching for %s in value", def)

	_def := internal.StringToDef(def)

	p.log.Debugf("%#v", _def)
	if !_def.IsImported() {
		// Look definition in current plan
		return p.instance.GetDefinition(_def.Def())
	} else {
		i, found := p.imports[_def.Pkg()]
		if !found {
			return nil, fmt.Errorf("imported package %s not registed in plan", _def.Def())
		}

		return i.GetDefinition(_def.Def())
	}
}

// Reload will rebuild the cue value
func (p *Plan) Reload() error {
	var (
		i   *loader.Instance
		v   *loader.Value
		err error
	)

	switch p.kind {
	case File:
		i, err = loader.File(p.rootPath, p.rootFilePath)
	case Directory:
		i, err = loader.Dir(p.rootPath, p.rootFilePath)
	}

	if err != nil {
		return err
	}

	v, err = i.GetValidatedValue()
	if err != nil {
		return err
	}

	p.instance = i
	p.v = v

	if err := p.loadImports(); err != nil {
		return err
	}

	if err := p.instance.LoadDefinitions(); err != nil {
		return err
	}

	return nil
}

// AddFile load and register a new file in the plan
// This file must be part of the instance
func (p *Plan) AddFile(path string) error {
	p.log.Debugf("Add a new file to plan: %s", path)

	f, err := file.New(path)
	if err != nil {
		return err
	}

	p.files[path] = f
	return nil
}