package generator

import (
	"bytes"
	"fmt"
	"go/types"
	"html/template"
	"log"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/types/typeutil"
	"golang.org/x/tools/imports"
)

// Fake is used to generate a Fake implementation of an interface.
type Fake struct {
	Packages           []*packages.Package
	Package            *packages.Package
	Interface          *types.TypeName
	DestinationPackage string
	Name               string
	InterfaceAlias     string
	InterfaceName      string
	InterfacePackage   string
	Imports            []Import
	Methods            []Method
}

// AddImport creates an import with the given alias and path, and adds it to
// Fake.Imports.
func (f *Fake) AddImport(alias string, path string) Import {
	path = unvendor(strings.TrimSpace(path))
	alias = strings.TrimSpace(alias)
	log.Printf("Adding import: %s > %s\n", alias, path)
	for i := range f.Imports {
		if f.Imports[i].Path == path {
			return f.Imports[i]
		}
	}

	result := Import{
		Alias: alias,
		Path:  path,
	}
	f.Imports = append(f.Imports, result)
	return result
}

// SortImports sorts imports alphabetically.
func (f *Fake) sortImports() {
	sort.SliceStable(f.Imports, func(i, j int) bool {
		return f.Imports[i].Path < f.Imports[j].Path
	})
}

func unvendor(s string) string {
	// Devendorize for use in import statement.
	if i := strings.LastIndex(s, "/vendor/"); i >= 0 {
		return s[i+len("/vendor/"):]
	}
	if strings.HasPrefix(s, "vendor/") {
		return s[len("vendor/"):]
	}
	return s
}

func (f *Fake) hasDuplicateAliases() bool {
	hasDuplicates := false
	for _, imports := range f.aliasMap() {
		if len(imports) > 1 {
			hasDuplicates = true
			break
		}
	}
	return hasDuplicates
}

func (f *Fake) printAliases() {
	for i := range f.Imports {
		log.Printf("- %s > %s\n", f.Imports[i].Alias, f.Imports[i].Path)
	}
}

// disambiguateAliases ensures that all imports are aliased uniquely.
func (f *Fake) disambiguateAliases() {
	f.sortImports()
	if !f.hasDuplicateAliases() {
		return
	}

	log.Printf("!!! Duplicate import aliases found,...")
	log.Printf("aliases before disambiguation:\n")
	f.printAliases()
	var byAlias map[string][]Import
	for {
		byAlias = f.aliasMap()
		if !f.hasDuplicateAliases() {
			break
		}

		for i := range f.Imports {
			imports := byAlias[f.Imports[i].Alias]
			if len(imports) == 1 {
				continue
			}

			for j := 0; j < len(imports); j++ {
				if imports[j].Path == f.Imports[i].Path && j > 0 {
					f.Imports[i].Alias = f.Imports[i].Alias + string('a'+byte(j-1))
					if f.Imports[i].Path == f.InterfacePackage {
						f.InterfaceAlias = f.Imports[i].Alias
					}
				}
			}
		}
	}

	log.Println("aliases after disambiguation:")
	f.printAliases()
}

func (f *Fake) aliasMap() map[string][]Import {
	result := map[string][]Import{}
	for i := range f.Imports {
		imports := result[f.Imports[i].Alias]
		result[f.Imports[i].Alias] = append(imports, f.Imports[i])
	}
	return result
}

func (f *Fake) importsMap() map[string]Import {
	f.disambiguateAliases()
	result := map[string]Import{}
	for i := range f.Imports {
		result[f.Imports[i].Path] = f.Imports[i]
	}
	return result
}

type Import struct {
	Alias string
	Path  string
}

type ByPath []Import

type Method struct {
	FakeName string
	Name     string
	Params   Params
	Args     string
	Returns  Returns
	Rets     string
}

type Params []Param

type Param struct {
	Name       string
	Type       string
	IsVariadic bool
	IsSlice    bool
}

func (p Params) Slices() Params {
	var result Params
	for i := range p {
		if p[i].IsSlice {
			result = append(result, p[i])
		}
	}
	return result
}

func (p Params) HasLength() bool {
	return len(p) > 0
}

type Returns []Return

type Return struct {
	Name string
	Type string
}

func (r Returns) HasLength() bool {
	return len(r) > 0
}

func (r Returns) WithPrefix(p string) string {
	if len(r) == 0 {
		return ""
	}

	rets := []string{}
	for i := range r {
		if p == "" {
			rets = append(rets, unexport(r[i].Name))
		} else {
			rets = append(rets, p+unexport(r[i].Name))
		}
	}
	return strings.Join(rets, ", ")
}

func (p Params) WithPrefix(prefix string) string {
	if len(p) == 0 {
		return ""
	}

	params := []string{}
	for i := range p {
		if prefix == "" {
			params = append(params, unexport(p[i].Name))
		} else {
			params = append(params, prefix+unexport(p[i].Name))
		}
	}
	return strings.Join(params, ", ")
}

func (r Returns) AsArgs() string {
	if len(r) == 0 {
		return ""
	}

	rets := []string{}
	for i := range r {
		log.Println(r[i].Type)
		rets = append(rets, r[i].Type)
	}
	return strings.Join(rets, ", ")
}

func (p Params) AsArgs() string {
	if len(p) == 0 {
		return ""
	}

	params := []string{}
	for i := range p {
		params = append(params, p[i].Type)
	}
	return strings.Join(params, ", ")
}

func (r Returns) AsNamedArgsWithTypes() string {
	if len(r) == 0 {
		return ""
	}

	rets := []string{}
	for i := range r {
		rets = append(rets, unexport(r[i].Name)+" "+r[i].Type)
	}
	return strings.Join(rets, ", ")
}

func (p Params) AsNamedArgsWithTypes() string {
	if len(p) == 0 {
		return ""
	}

	params := []string{}
	for i := range p {
		params = append(params, unexport(p[i].Name)+" "+p[i].Type)
	}
	return strings.Join(params, ", ")
}

func (r Returns) AsNamedArgs() string {
	if len(r) == 0 {
		return ""
	}

	rets := []string{}
	for i := range r {
		rets = append(rets, unexport(r[i].Name))
	}
	return strings.Join(rets, ", ")
}

func (p Params) AsNamedArgs() string {
	if len(p) == 0 {
		return ""
	}

	params := []string{}
	for i := range p {
		if p[i].IsSlice {
			params = append(params, unexport(p[i].Name)+"Copy")
		} else {
			params = append(params, unexport(p[i].Name))
		}
	}
	return strings.Join(params, ", ")
}

func (p Params) AsNamedArgsForInvocation() string {
	if len(p) == 0 {
		return ""
	}

	params := []string{}
	for i := range p {
		if p[i].IsVariadic {
			params = append(params, unexport(p[i].Name)+"...")
		} else {
			params = append(params, unexport(p[i].Name))
		}
	}
	return strings.Join(params, ", ")
}

func (p Params) AsReturnSignature() string {
	if len(p) == 0 {
		return ""
	}
	if len(p) == 1 {
		if p[0].IsVariadic {
			return strings.Replace(p[0].Type, "...", "[]", -1)
		}
		return p[0].Type
	}
	result := "("
	for i := range p {
		t := p[i].Type
		if p[i].IsVariadic {
			t = strings.Replace(t, "...", "[]", -1)
		}
		result = result + t
		if i < len(p) {
			result = result + ", "
		}
	}
	result = result + ")"
	return result
}

func (r Returns) AsReturnSignature() string {
	if len(r) == 0 {
		return ""
	}
	if len(r) == 1 {
		return r[0].Type
	}
	result := "("
	for i := range r {
		result = result + r[i].Type
		if i < len(r) {
			result = result + ", "
		}
	}
	result = result + ")"
	return result
}

func unexport(s string) string {
	if s == "" {
		return ""
	}
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToLower(r)) + s[n:]
}

var funcs template.FuncMap = template.FuncMap{
	"ToLower":  strings.ToLower,
	"UnExport": unexport,
	"Replace":  strings.Replace,
}

func (f *Fake) loadPackages(packagePath string) error {
	p, err := packages.Load(&packages.Config{
		Mode: packages.LoadSyntax,
	}, packagePath)
	if err != nil {
		return err
	}
	f.Packages = p
	return nil
}

func (f *Fake) findPackageWithInterface() error {
	var iface *types.TypeName
	var pkg *packages.Package
	for i := range f.Packages {
		if f.Packages[i].Types == nil || f.Packages[i].Types.Scope() == nil {
			continue
		}
		pkg = f.Packages[i]

		raw := pkg.Types.Scope().Lookup(f.InterfaceName)
		if raw != nil {
			if typeName, ok := raw.(*types.TypeName); ok {
				iface = typeName
				break
			}
		}
	}
	if pkg == nil || iface == nil {
		return fmt.Errorf("cannot find package with interface %s", f.InterfaceName)
	}
	f.Interface = iface
	f.Package = pkg
	f.InterfaceName = iface.Name()
	f.InterfacePackage = unvendor(pkg.PkgPath)
	f.InterfaceAlias = pkg.Name
	f.AddImport(f.InterfaceAlias, f.InterfacePackage)
	return nil
}

func NewFake(interfaceName string, packagePath string, fakeName string, destinationPackage string) (*Fake, error) {
	f := &Fake{
		InterfaceName:      interfaceName,
		InterfacePackage:   packagePath,
		Name:               fakeName,
		DestinationPackage: destinationPackage,
		Imports: []Import{
			Import{
				Alias: "sync",
				Path:  "sync",
			},
		},
	}

	err := f.loadPackages(packagePath)
	if err != nil {
		return nil, err
	}

	err = f.findPackageWithInterface()
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (f *Fake) Generate(runImports bool) ([]byte, error) {
	log.Printf("Writing fake %s for interface %s to package %s\n", f.Name, f.InterfaceName, f.DestinationPackage)
	methods := typeutil.IntuitiveMethodSet(f.Interface.Type(), nil)
	for i := range methods {
		sig := methods[i].Type().(*types.Signature)
		log.Printf("Preparing method %s...", methods[i].String())
		for i := 0; i < sig.Results().Len(); i++ {
			ret := sig.Results().At(i)
			f.addImportsFor(ret.Type())
		}
		for i := 0; i < sig.Params().Len(); i++ {
			param := sig.Params().At(i)
			f.addImportsFor(param.Type())
		}
	}

	importsMap := f.importsMap()
	for i := range methods {
		sig := methods[i].Type().(*types.Signature)
		fun := methods[i].Obj().(*types.Func)
		params := []Param{}
		for i := 0; i < sig.Params().Len(); i++ {
			param := sig.Params().At(i)
			isVariadic := i == sig.Params().Len()-1 && sig.Variadic()
			typ := typeFor(param.Type(), importsMap)
			if isVariadic {
				typ = "..." + typ[2:] // Change []string to ...string
			}
			p := Param{
				Name:       fmt.Sprintf("arg%v", i+1),
				Type:       typ,
				IsVariadic: isVariadic,
				IsSlice:    strings.HasPrefix(typ, "[]"),
			}
			params = append(params, p)
		}
		returns := []Return{}
		for i := 0; i < sig.Results().Len(); i++ {
			ret := sig.Results().At(i)
			r := Return{
				Name: fmt.Sprintf("result%v", i+1),
				Type: typeFor(ret.Type(), importsMap),
			}
			returns = append(returns, r)
		}
		method := Method{
			FakeName: f.Name,
			Name:     fun.Name(),
			Returns:  returns,
			Params:   params,
		}
		f.Methods = append(f.Methods, method)
	}

	// Generate the template
	tmpl := template.Must(template.New("fake").Funcs(funcs).Parse(fakeTemplate))
	b := &bytes.Buffer{}
	tmpl.Execute(b, f)
	if runImports {
		return imports.Process("counterfeiter_temp_process_file", b.Bytes(), nil)
	}
	return b.Bytes(), nil
}

func typeFor(typ types.Type, importsMap map[string]Import) string {
	if typ == nil {
		return ""
	}
	switch t := typ.(type) {
	case *types.Slice:
		return "[]" + typeFor(t.Elem(), importsMap)
	case *types.Array:
		return fmt.Sprintf("[%v]%s", t.Len(), typeFor(t.Elem(), importsMap))
	case *types.Pointer:
		return "*" + typeFor(t.Elem(), importsMap)
	case *types.Map:
		return "map[" + typeFor(t.Key(), importsMap) + "]" + typeFor(t.Elem(), importsMap)
	case *types.Chan:
		switch t.Dir() {
		case types.SendRecv:
			return "chan " + typeFor(t.Elem(), importsMap)
		case types.SendOnly:
			return "chan<- " + typeFor(t.Elem(), importsMap)
		case types.RecvOnly:
			return "<-chan " + typeFor(t.Elem(), importsMap)
		}

	case *types.Basic:
		return t.Name()
	case *types.Named:
		if t.Obj() == nil {
			log.Println(t.String())
			return ""
		}
		if t.Obj().Pkg() == nil {
			return t.Obj().Name()
		}
		imp := importsMap[unvendor(t.Obj().Pkg().Path())]
		if imp.Path == "" {
			return t.Obj().Name()
		}

		return imp.Alias + "." + t.Obj().Name()
	default:
		log.Printf("!!! WARNING: Missing case for type %s\n", reflect.TypeOf(typ).String())
	}

	return ""
}

// addImportsFor inspects the given type and adds imports to the fake if importable
// types are found.
func (f *Fake) addImportsFor(typ types.Type) {
	if typ == nil {
		return
	}

	switch t := typ.(type) {
	case *types.Basic:
		return
	case *types.Pointer:
		f.addImportsFor(t.Elem())
	case *types.Map:
		f.addImportsFor(t.Key())
		f.addImportsFor(t.Elem())
	case *types.Chan:
		f.addImportsFor(t.Elem())
	case *types.Named:
		if t.Obj() != nil && t.Obj().Pkg() != nil {
			f.AddImport(t.Obj().Pkg().Name(), t.Obj().Pkg().Path())
		}
	case *types.Slice:
		f.addImportsFor(t.Elem())
	case *types.Array:
		f.addImportsFor(t.Elem())
	default:
		log.Printf("!!! WARNING: Missing case for type %s\n", reflect.TypeOf(typ).String())
	}
}

const fakeTemplate string = `// Code generated by counterfeiter. DO NOT EDIT.
package {{.DestinationPackage}}

import (
	{{range .Imports}}{{.Alias}} "{{.Path}}"
	{{end}}
)

type {{.Name}} struct {
	{{range .Methods}}{{.Name}}Stub func({{.Params.AsArgs}}) {{.Returns.AsReturnSignature}}
	{{UnExport .Name}}Mutex sync.RWMutex
	{{UnExport .Name}}ArgsForCall []struct{ {{- range .Params}}
		{{.Name}} {{if .IsVariadic}}{{Replace .Type "..." "[]" -1}}{{else}}{{.Type}}{{end}}
	{{end -}} }
	{{if .Returns.HasLength}}{{UnExport .Name}}Returns struct{
		{{range .Returns}}{{UnExport .Name}} {{.Type}}
		{{end}}
	}
	{{UnExport .Name}}ReturnsOnCall map[int]struct{
		{{range .Returns}}{{UnExport .Name}} {{.Type}}
		{{end}}
	}{{end}}
	{{end}}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}{{range .Methods}}

func (fake *{{.FakeName}}) {{.Name}}({{.Params.AsNamedArgsWithTypes}}) {{.Returns.AsReturnSignature}} {
	{{range .Params.Slices}}var {{UnExport .Name}}Copy {{.Type}}
	if {{UnExport .Name}} != nil {
		{{UnExport .Name}}Copy = make({{.Type}}, len({{UnExport .Name}}))
		copy({{UnExport .Name}}Copy, {{UnExport .Name}})
	}
	{{end}}fake.{{UnExport .Name}}Mutex.Lock()
	{{if .Returns.HasLength}}ret, specificReturn := fake.{{UnExport .Name}}ReturnsOnCall[len(fake.{{UnExport .Name}}ArgsForCall)]
	{{end}}fake.{{UnExport .Name}}ArgsForCall = append(fake.{{UnExport .Name}}ArgsForCall, struct{ {{- range .Params}}
		{{.Name}} {{if .IsVariadic}}{{Replace .Type "..." "[]" -1}}{{else}}{{.Type}}{{end}}
	{{end -}} }{ {{- .Params.AsNamedArgs -}} })
	fake.recordInvocation("{{.Name}}", []interface{}{ {{- if .Params.HasLength}}{{.Params.AsNamedArgs}}{{end -}} })
	fake.{{UnExport .Name}}Mutex.Unlock()
	if fake.{{.Name}}Stub != nil {
		{{if .Returns.HasLength}}return fake.{{.Name}}Stub({{.Params.AsNamedArgsForInvocation}}){{else}}fake.{{.Name}}Stub({{.Params.AsNamedArgsForInvocation}}){{end}}
	}{{if .Returns.HasLength}}
	if specificReturn {
		return {{.Returns.WithPrefix "ret."}}
	}
	fakeReturns := fake.{{UnExport .Name}}Returns
	return {{.Returns.WithPrefix "fakeReturns."}}{{end}}
}

func (fake *{{.FakeName}}) {{.Name}}CallCount() int {
	fake.{{UnExport .Name}}Mutex.RLock()
	defer fake.{{UnExport .Name}}Mutex.RUnlock()
	return len(fake.{{UnExport .Name}}ArgsForCall)
}

{{if .Params.HasLength}}func (fake *{{.FakeName}}) {{.Name}}ArgsForCall(i int) {{.Params.AsReturnSignature}} {
	fake.{{UnExport .Name}}Mutex.RLock()
	defer fake.{{UnExport .Name}}Mutex.RUnlock()
	argsForCall := fake.{{UnExport .Name}}ArgsForCall[i]
	return {{.Params.WithPrefix "argsForCall."}}
}{{end}}

{{if .Returns.HasLength}}func (fake *{{.FakeName}}) {{.Name}}Returns({{.Returns.AsNamedArgsWithTypes}}) {
	fake.{{.Name}}Stub = nil
	fake.{{UnExport .Name}}Returns = struct {
		{{range .Returns}}{{UnExport .Name}} {{.Type}}
		{{end}}
	}{ {{- .Returns.AsNamedArgs -}} }
}

func (fake *{{.FakeName}}) {{.Name}}ReturnsOnCall(i int, {{.Returns.AsNamedArgsWithTypes}}) {
	fake.{{.Name}}Stub = nil
	if fake.{{UnExport .Name}}ReturnsOnCall == nil {
		fake.{{UnExport .Name}}ReturnsOnCall = make(map[int]struct {
			{{range .Returns}}{{UnExport .Name}} {{.Type}}
			{{end}}
		})
	}
	fake.{{UnExport .Name}}ReturnsOnCall[i] = struct {
		{{range .Returns}}{{UnExport .Name}} {{.Type}}
		{{end}}
	}{ {{- .Returns.AsNamedArgs -}} }
}{{end}}

{{end}}func (fake *{{.Name}}) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	{{range .Methods}}fake.{{UnExport .Name}}Mutex.RLock()
	defer fake.{{UnExport .Name}}Mutex.RUnlock()
	{{end}}copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *{{.Name}}) recordInvocation(key string, args []interface{}) {
	fake.invocationsMutex.Lock()
	defer fake.invocationsMutex.Unlock()
	if fake.invocations == nil {
		fake.invocations = map[string][][]interface{}{}
	}
	if fake.invocations[key] == nil {
		fake.invocations[key] = [][]interface{}{}
	}
	fake.invocations[key] = append(fake.invocations[key], args)
}

var _ {{.InterfaceAlias}}.{{.InterfaceName}} = new({{.Name}})
`
