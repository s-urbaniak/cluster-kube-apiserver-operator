// Copyright ©2016 The Gonum Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This repository is no longer maintained.
// Development has moved to https://github.com/gonum/gonum.
//
// Package binding provides helpers for building autogenerated cgo bindings.
package binding

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"text/template"
	"unsafe"

	"github.com/cznic/cc"
	"github.com/cznic/xc"
)

func model() *cc.Model {
	p := int(unsafe.Sizeof(uintptr(0)))
	i := int(unsafe.Sizeof(int(0)))
	return &cc.Model{
		Items: map[cc.Kind]cc.ModelItem{
			cc.Ptr:               {Size: p, Align: p, StructAlign: p},
			cc.UintPtr:           {Size: p, Align: p, StructAlign: p},
			cc.Void:              {Size: 0, Align: 1, StructAlign: 1},
			cc.Char:              {Size: 1, Align: 1, StructAlign: 1},
			cc.SChar:             {Size: 1, Align: 1, StructAlign: 1},
			cc.UChar:             {Size: 1, Align: 1, StructAlign: 1},
			cc.Short:             {Size: 2, Align: 2, StructAlign: 2},
			cc.UShort:            {Size: 2, Align: 2, StructAlign: 2},
			cc.Int:               {Size: 4, Align: 4, StructAlign: 4},
			cc.UInt:              {Size: 4, Align: 4, StructAlign: 4},
			cc.Long:              {Size: i, Align: i, StructAlign: i},
			cc.ULong:             {Size: i, Align: i, StructAlign: i},
			cc.LongLong:          {Size: 8, Align: 8, StructAlign: 8},
			cc.ULongLong:         {Size: 8, Align: 8, StructAlign: 8},
			cc.Float:             {Size: 4, Align: 4, StructAlign: 4},
			cc.Double:            {Size: 8, Align: 8, StructAlign: 8},
			cc.LongDouble:        {Size: 8, Align: 8, StructAlign: 8},
			cc.Bool:              {Size: 1, Align: 1, StructAlign: 1},
			cc.FloatComplex:      {Size: 8, Align: 8, StructAlign: 8},
			cc.DoubleComplex:     {Size: 16, Align: 16, StructAlign: 16},
			cc.LongDoubleComplex: {Size: 16, Align: 16, StructAlign: 16},
		},
	}
}

// TypeKey is a terse C type description.
type TypeKey struct {
	IsPointer bool
	Kind      cc.Kind
}

var goTypes = map[TypeKey]*template.Template{
	{Kind: cc.Undefined}:               template.Must(template.New("<undefined>").Parse("<undefined>")),
	{Kind: cc.Int}:                     template.Must(template.New("int").Parse("int")),
	{Kind: cc.Float}:                   template.Must(template.New("float32").Parse("float32")),
	{Kind: cc.Float, IsPointer: true}:  template.Must(template.New("[]float32").Parse("[]float32")),
	{Kind: cc.Double}:                  template.Must(template.New("float64").Parse("float64")),
	{Kind: cc.Double, IsPointer: true}: template.Must(template.New("[]float64").Parse("[]float64")),
	{Kind: cc.Bool}:                    template.Must(template.New("bool").Parse("bool")),
	{Kind: cc.FloatComplex}:            template.Must(template.New("complex64").Parse("complex64")),
	{Kind: cc.DoubleComplex}:           template.Must(template.New("complex128").Parse("complex128")),
}

// GoTypeFor returns a string representation of the given type using a mapping in
// types. GoTypeFor will panic if no type mapping is found after searching the
// user-provided types mappings and then the following mapping:
//  {Kind: cc.Int}:                     "int",
//  {Kind: cc.Float}:                   "float32",
//  {Kind: cc.Float, IsPointer: true}:  "[]float32",
//  {Kind: cc.Double}:                  "float64",
//  {Kind: cc.Double, IsPointer: true}: "[]float64",
//  {Kind: cc.Bool}:                    "bool",
//  {Kind: cc.FloatComplex}:            "complex64",
//  {Kind: cc.DoubleComplex}:           "complex128",
func GoTypeFor(typ cc.Type, name string, types ...map[TypeKey]*template.Template) string {
	if typ == nil {
		return "<nil>"
	}
	k := typ.Kind()
	isPtr := typ.Kind() == cc.Ptr
	if isPtr {
		k = typ.Element().Kind()
	}
	var buf bytes.Buffer
	for _, t := range types {
		if s, ok := t[TypeKey{Kind: k, IsPointer: isPtr}]; ok {
			err := s.Execute(&buf, name)
			if err != nil {
				panic(err)
			}
			return buf.String()
		}
	}
	s, ok := goTypes[TypeKey{Kind: k, IsPointer: isPtr}]
	if ok {
		err := s.Execute(&buf, name)
		if err != nil {
			panic(err)
		}
		return buf.String()
	}
	panic(fmt.Sprintf("unknown type key: %+v", TypeKey{Kind: k, IsPointer: isPtr}))
}

// GoTypeForEnum returns a string representation of the given enum type using a mapping
// in types. GoTypeForEnum will panic if no type mapping is found after searching the
// user-provided types mappings or the type is not an enum.
func GoTypeForEnum(typ cc.Type, name string, types ...map[string]*template.Template) string {
	if typ == nil {
		return "<nil>"
	}
	if typ.Kind() != cc.Enum {
		panic(fmt.Sprintf("invalid type: %v", typ))
	}
	tag := typ.Tag()
	if tag != 0 {
		n := string(xc.Dict.S(tag))
		for _, t := range types {
			if s, ok := t[n]; ok {
				var buf bytes.Buffer
				err := s.Execute(&buf, name)
				if err != nil {
					panic(err)
				}
				return buf.String()
			}
		}
	}
	panic(fmt.Sprintf("unknown type: %+v", typ))
}

var cgoTypes = map[TypeKey]*template.Template{
	{Kind: cc.Void, IsPointer: true}: template.Must(template.New("void*").Parse("unsafe.Pointer(&{{.}}[0])")),

	{Kind: cc.Int}: template.Must(template.New("int").Parse("C.int({{.}})")),

	{Kind: cc.Float}:  template.Must(template.New("float").Parse("C.float({{.}})")),
	{Kind: cc.Double}: template.Must(template.New("double").Parse("C.double({{.}})")),

	{Kind: cc.Float, IsPointer: true}:  template.Must(template.New("float*").Parse("(*C.float)(&{{.}}[0])")),
	{Kind: cc.Double, IsPointer: true}: template.Must(template.New("double*").Parse("(*C.double)(&{{.}}[0])")),

	{Kind: cc.Bool}: template.Must(template.New("bool").Parse("C.bool({{.}})")),

	{Kind: cc.FloatComplex}:                   template.Must(template.New("floatcomplex").Parse("unsafe.Pointer({{.}})")),
	{Kind: cc.DoubleComplex}:                  template.Must(template.New("doublecomplex").Parse("unsafe.Pointer({{.}})")),
	{Kind: cc.FloatComplex, IsPointer: true}:  template.Must(template.New("floatcomplex*").Parse("unsafe.Pointer(&{{.}}[0])")),
	{Kind: cc.DoubleComplex, IsPointer: true}: template.Must(template.New("doublecomplex*").Parse("unsafe.Pointer(&{{.}}[0])")),
}

// CgoConversionFor returns a string representation of the given type using a mapping in
// types. CgoConversionFor will panic if no type mapping is found after searching the
// user-provided types mappings and then the following mapping:
//  {Kind: cc.Void, IsPointer: true}:          "unsafe.Pointer(&{{.}}[0])",
//  {Kind: cc.Int}:                            "C.int({{.}})",
//  {Kind: cc.Float}:                          "C.float({{.}})",
//  {Kind: cc.Float, IsPointer: true}:         "(*C.float)({{.}})",
//  {Kind: cc.Double}:                         "C.double({{.}})",
//  {Kind: cc.Double, IsPointer: true}:        "(*C.double)({{.}})",
//  {Kind: cc.Bool}:                           "C.bool({{.}})",
//  {Kind: cc.FloatComplex}:                   "unsafe.Pointer(&{{.}})",
//  {Kind: cc.DoubleComplex}:                  "unsafe.Pointer(&{{.}})",
//  {Kind: cc.FloatComplex, IsPointer: true}:  "unsafe.Pointer(&{{.}}[0])",
//  {Kind: cc.DoubleComplex, IsPointer: true}: "unsafe.Pointer(&{{.}}[0])",
func CgoConversionFor(name string, typ cc.Type, types ...map[TypeKey]*template.Template) string {
	if typ == nil {
		return "<nil>"
	}
	k := typ.Kind()
	isPtr := typ.Kind() == cc.Ptr
	if isPtr {
		k = typ.Element().Kind()
	}
	for _, t := range types {
		if s, ok := t[TypeKey{Kind: k, IsPointer: isPtr}]; ok {
			var buf bytes.Buffer
			err := s.Execute(&buf, name)
			if err != nil {
				panic(err)
			}
			return buf.String()
		}
	}
	s, ok := cgoTypes[TypeKey{Kind: k, IsPointer: isPtr}]
	if ok {
		var buf bytes.Buffer
		err := s.Execute(&buf, name)
		if err != nil {
			panic(err)
		}
		return buf.String()
	}
	panic(fmt.Sprintf("unknown type key: %+v", TypeKey{Kind: k, IsPointer: isPtr}))
}

// CgoConversionForEnum returns a string representation of the given enum type using a mapping
// in types. GoTypeForEnum will panic if no type mapping is found after searching the
// user-provided types mappings or the type is not an enum.
func CgoConversionForEnum(name string, typ cc.Type, types ...map[string]*template.Template) string {
	if typ == nil {
		return "<nil>"
	}
	if typ.Kind() != cc.Enum {
		panic(fmt.Sprintf("invalid type: %v", typ))
	}
	tag := typ.Tag()
	if tag != 0 {
		n := string(xc.Dict.S(tag))
		for _, t := range types {
			if s, ok := t[n]; ok {
				var buf bytes.Buffer
				err := s.Execute(&buf, name)
				if err != nil {
					panic(err)
				}
				return buf.String()
			}
		}
	}
	panic(fmt.Sprintf("unknown type: %+v", typ))
}

// LowerCaseFirst returns s with the first character lower-cased. LowerCaseFirst
// assumes s is an ASCII-represented string.
func LowerCaseFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]|' ') + s[1:]
}

// UpperCaseFirst returns s with the first character upper-cased. UpperCaseFirst
// assumes s is an ASCII-represented string.
func UpperCaseFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]&^' ') + s[1:]
}

// DocComments returns a map of method documentation comments for the package at the
// given path. The first key of the returned map is the type name and the second
// is the method name. Non-method function documentation are in docs[""].
func DocComments(path string) (docs map[string]map[string][]*ast.Comment, err error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	docs = make(map[string]map[string][]*ast.Comment)
	for _, p := range pkgs {
		for _, f := range p.Files {
			for _, n := range f.Decls {
				fn, ok := n.(*ast.FuncDecl)
				if !ok || fn.Doc == nil {
					continue
				}

				var typ string
				if fn.Recv != nil && len(fn.Recv.List) > 0 {
					id, ok := fn.Recv.List[0].Type.(*ast.Ident)
					if ok {
						typ = id.Name
					}
				}
				doc, ok := docs[typ]
				if !ok {
					doc = make(map[string][]*ast.Comment)
					docs[typ] = doc
				}
				doc[fn.Name.String()] = fn.Doc.List
			}
		}
	}

	return docs, nil
}

// Declaration is a description of a C function declaration.
type Declaration struct {
	Pos         token.Pos
	Name        string
	Return      cc.Type
	CParameters []cc.Parameter
	Variadic    bool
}

// Position returns the token position of the declaration.
func (d *Declaration) Position() token.Position { return xc.FileSet.Position(d.Pos) }

// Parameter is a C function parameter.
type Parameter struct{ Parameter cc.Parameter }

// Name returns the name of the parameter.
func (p *Parameter) Name() string { return string(xc.Dict.S(p.Parameter.Name)) }

// Type returns the C type of the parameter.
func (p *Parameter) Type() cc.Type { return p.Parameter.Type }

// Kind returns the C kind of the parameter.
func (p *Parameter) Kind() cc.Kind { return p.Parameter.Type.Kind() }

// Elem returns the pointer type of a pointer parameter or the element type of an
// array parameter.
func (p *Parameter) Elem() cc.Type { return p.Parameter.Type.Element() }

// Parameters returns the declaration's CParameters converted to a []Parameter.
func (d *Declaration) Parameters() []Parameter {
	p := make([]Parameter, len(d.CParameters))
	for i, c := range d.CParameters {
		p[i] = Parameter{c}
	}
	return p
}

// Declarations returns the C function declarations in the givel set of file paths.
func Declarations(paths ...string) ([]Declaration, error) {
	predefined, includePaths, sysIncludePaths, err := cc.HostConfig()
	if err != nil {
		return nil, fmt.Errorf("binding: failed to get host config: %v", err)
	}

	t, err := cc.Parse(
		predefined+`
#define __const const
#define __attribute__(...)
#define __extension__
#define __inline
#define __restrict
unsigned __builtin_bswap32 (unsigned x);
unsigned long long __builtin_bswap64 (unsigned long long x);
`,
		paths,
		model(),
		cc.IncludePaths(includePaths),
		cc.SysIncludePaths(sysIncludePaths),
	)
	if err != nil {
		return nil, fmt.Errorf("binding: failed to parse %q: %v", paths, err)
	}

	var decls []Declaration
	for ; t != nil; t = t.TranslationUnit {
		if t.ExternalDeclaration.Case != 1 /* Declaration */ {
			continue
		}

		d := t.ExternalDeclaration.Declaration
		if d.Case != 0 {
			// Other case is 1: StaticAssertDeclaration.
			continue
		}

		init := d.InitDeclaratorListOpt
		if init == nil {
			continue
		}
		idl := init.InitDeclaratorList
		if idl.InitDeclaratorList != nil {
			// We do not want comma-separated lists.
			continue
		}
		id := idl.InitDeclarator
		if id.Case != 0 {
			// We do not want assignments.
			continue
		}

		declarator := id.Declarator
		if declarator.Type.Kind() != cc.Function {
			// We want only functions.
			continue
		}
		params, variadic := declarator.Type.Parameters()
		name, _ := declarator.Identifier()
		decls = append(decls, Declaration{
			Pos:         declarator.Pos(),
			Name:        string(xc.Dict.S(name)),
			Return:      declarator.Type.Result(),
			CParameters: params,
			Variadic:    variadic,
		})
	}

	sort.Sort(byPosition(decls))

	return decls, nil
}

type byPosition []Declaration

func (d byPosition) Len() int { return len(d) }
func (d byPosition) Less(i, j int) bool {
	iPos := d[i].Position()
	jPos := d[j].Position()
	if iPos.Filename == jPos.Filename {
		return iPos.Line < jPos.Line
	}
	return iPos.Filename < jPos.Filename
}
func (d byPosition) Swap(i, j int) { d[i], d[j] = d[j], d[i] }
