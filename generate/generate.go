//go:build ignore

package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/iancoleman/strcase"
)

var EnumStringTypes []string

func main() {
	// TODO: actually host the spec here.
	/*uri := "https://api.oxide.computer"
	u, err := url.Parse(uri)
	if err != nil {
		fmt.Printf("error parsing url %q: %v\n", uri, err)
		os.Exit(1)
	}

	// Load the open API spec from the URI.
	doc, err := openapi3.NewLoader().LoadFromURI(u)
	if err != nil {
		fmt.Printf("error loading openAPI spec from %q: %v\n", uri, err)
		os.Exit(1)
	}*/

	// Load the open API spec from the file.
	wd, err := os.Getwd()
	if err != nil {
		fmt.Printf("error getting current working directory: %v\n", err)
		os.Exit(1)
	}
	p := filepath.Join(wd, "spec.json")
	doc, err := openapi3.NewLoader().LoadFromFile(p)
	if err != nil {
		fmt.Printf("error loading openAPI spec from file %q: %v\n", p, err)
		os.Exit(1)
	}

	// Generate the types.go file.
	generateTypes(doc)

	// Generate the responses.go file.
	generateResponses(doc)

	// Generate the paths.go file.
	generatePaths(doc)
}

// Generate the types.go file.
func generateTypes(doc *openapi3.T) {
	f := openGeneratedFile("types.go")
	defer f.Close()

	// Iterate over all the schema components in the spec and write the types.
	// We want to ensure we keep the order so the diffs don't look like shit.
	keys := make([]string, 0)
	for k := range doc.Components.Schemas {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		s := doc.Components.Schemas[name]
		if s.Ref != "" {
			fmt.Printf("[WARN] TODO: skipping type for %q, since it is a reference\n", name)
			continue
		}

		writeSchemaType(f, name, s.Value, "")
	}
}

// Generate the responses.go file.
func generateResponses(doc *openapi3.T) {
	f := openGeneratedFile("responses.go")
	defer f.Close()

	// Iterate over all the responses in the spec and write the types.
	// We want to ensure we keep the order so the diffs don't look like shit.
	keys := make([]string, 0)
	for k := range doc.Components.Responses {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		r := doc.Components.Responses[name]
		if r.Ref != "" {
			fmt.Printf("[WARN] TODO: skipping response for %q, since it is a reference\n", name)
			continue
		}

		writeResponseType(f, name, r.Value)
	}
}

// Generate the paths.go file.
func generatePaths(doc *openapi3.T) {
	f := openGeneratedFile("paths.go")
	defer f.Close()

	// Iterate over all the paths in the spec and write the types.
	// We want to ensure we keep the order so the diffs don't look like shit.
	keys := make([]string, 0)
	for k := range doc.Paths {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, path := range keys {
		p := doc.Paths[path]
		if p.Ref != "" {
			fmt.Printf("[WARN] TODO: skipping path for %q, since it is a reference\n", path)
			continue
		}

		writePath(f, path, p)
	}
}

func openGeneratedFile(filename string) *os.File {
	// Get the current working directory.
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("error getting current working directory: %v\n", err)
		os.Exit(1)
	}

	p := filepath.Join(cwd, filename)

	// Create the types.go file.
	// Open the file for writing.
	f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		fmt.Printf("error creating %q: %v\n", p, err)
		os.Exit(1)
	}

	// Add the header to the package.
	fmt.Fprintf(f, "// Code generated by `%s`. DO NOT EDIT.\n\n", filepath.Base(os.Args[0]))
	fmt.Fprintln(f, "package oxide")
	fmt.Fprintln(f, "")

	return f
}

// printProperty converts an object's property name to a valid Go identifier.
func printProperty(p string) string {
	c := strcase.ToCamel(p)
	if c == "Id" {
		c = "ID"
	} else if c == "IpAddress" {
		c = "IPAddress"
	} else if c == "UserId" {
		c = "UserID"
	} else if c == "IdSortMode" {
		c = "IDSortMode"
	} else if strings.HasPrefix(c, "Cpu") {
		c = strings.Replace(c, "Cpu", "CPU", 1)
	} else if strings.HasPrefix(c, "Vpc") {
		c = strings.Replace(c, "Vpc", "VPC", 1)
	} else if strings.HasPrefix(c, "Vpn") {
		c = strings.Replace(c, "Vpn", "VPN", 1)
	} else if strings.HasPrefix(c, "Ipv4") {
		c = strings.Replace(c, "Ipv4", "IPv4", 1)
	} else if strings.HasPrefix(c, "Ipv6") {
		c = strings.Replace(c, "Ipv6", "IPv6", 1)
	}
	return c
}

// printType converts a schema type to a valid Go type.
func printType(property string, r *openapi3.SchemaRef) string {
	s := r.Value
	t := s.Type

	// If we have a reference, just use that.
	if r.Ref != "" {
		return getReferenceSchema(r)
	}

	// See if we have an allOf.
	if s.AllOf != nil {
		if len(s.AllOf) > 1 {
			fmt.Printf("[WARN] TODO: allOf for %q has more than 1 item\n", property)
			return "TODO"
		}

		return printType(property, s.AllOf[0])
	}

	if t == "string" {
		reference := getReferenceSchema(r)
		if reference != "" {
			return reference
		}

		return formatStringType(s)
	} else if t == "integer" {
		return "int"
	} else if t == "number" {
		return "float64"
	} else if t == "boolean" {
		return "bool"
	} else if t == "array" {
		reference := getReferenceSchema(s.Items)
		if reference != "" {
			return fmt.Sprintf("[]%s", reference)
		}

		// TODO: handle if it is not a reference.
		return "[]TODO"
	} else if t == "object" {
		// Most likely this is a local object, we will handle it.
		return strcase.ToCamel(property)
	}

	fmt.Printf("[WARN] TODO: skipping type %q for %q, marking as interface{}\n", t, property)
	return "interface{}"
}

// writePath writes the given path as an http request to the given file.
func writePath(f *os.File, path string, p *openapi3.PathItem) {
	if p.Get != nil {
		writeMethod(f, http.MethodGet, path, p.Get)
	}

	if p.Post != nil {
		writeMethod(f, http.MethodPost, path, p.Post)
	}

	if p.Put != nil {
		writeMethod(f, http.MethodPut, path, p.Put)
	}

	if p.Delete != nil {
		writeMethod(f, http.MethodDelete, path, p.Delete)
	}

	if p.Patch != nil {
		writeMethod(f, http.MethodPatch, path, p.Patch)
	}

	if p.Head != nil {
		writeMethod(f, http.MethodHead, path, p.Head)
	}

	if p.Options != nil {
		writeMethod(f, http.MethodOptions, path, p.Options)
	}
}

func writeMethod(f *os.File, method string, path string, o *openapi3.Operation) {
	respType := getSuccessResponseType(o)
	fnName := strcase.ToCamel(o.OperationID)

	// Parse the parameters.
	params := map[string]*openapi3.Parameter{}
	paramsString := ""
	for _, p := range o.Parameters {
		if p.Ref != "" {
			fmt.Printf("[WARN] TODO: skipping parameter for %q, since it is a reference\n", p.Value.Name)
			continue
		}

		params[p.Value.Name] = p.Value
		paramsString += fmt.Sprintf("%s %s, ", strcase.ToLowerCamel(p.Value.Name), printType(p.Value.Name, p.Value.Schema))
	}

	// Parse the request body.
	reqBodyParam := "nil"
	reqBodyDescription := ""
	if o.RequestBody != nil {
		rb := o.RequestBody

		if rb.Value.Description != "" {
			reqBodyDescription = rb.Value.Description
		}

		if rb.Ref != "" {
			fmt.Printf("[WARN] TODO: skipping request body for %q, since it is a reference: %q\n", path, rb.Ref)
		}

		for mt, r := range rb.Value.Content {
			if mt != "application/json" {
				paramsString += "b io.Reader"
				reqBodyParam = "b"
				break
			}

			paramsString += "j *" + printType("", r.Schema)
			reqBodyParam = "j"
			break
		}

	}

	fmt.Printf("writing method %q for path %q\n", method, path)

	// Write the description for the method.
	fmt.Fprintf(f, "// %s: %s\n", fnName, o.Summary)
	if o.Description != "" {
		fmt.Fprintln(f, "//")
		fmt.Fprintf(f, "// %s\n", strings.ReplaceAll(o.Description, "\n", "\n// "))
	}
	if len(params) > 0 {
		fmt.Fprintf(f, "//\n// Parameters:\n")
		for name, t := range params {
			if t.Description != "" {
				fmt.Fprintf(f, "//\t`%s`: %s\n", strcase.ToLowerCamel(name), strings.ReplaceAll(t.Description, "\n", "\n//\t\t"))
			}
		}
	}

	if reqBodyDescription != "" && reqBodyParam != "nil" {
		fmt.Fprintf(f, "//\t`%s`: %s\n", reqBodyParam, strings.ReplaceAll(reqBodyDescription, "\n", "\n// "))
	}

	// Write the method.
	if respType != "" {
		fmt.Fprintf(f, "func (c *Client) %s(%s) (*%s, error) {\n",
			fnName,
			paramsString,
			respType)
	} else {
		fmt.Fprintf(f, "func (c *Client) %s(%s) (error) {\n",
			fnName,
			paramsString)
	}

	// Create the url.
	fmt.Fprintln(f, "// Create the url.")
	fmt.Fprintf(f, "path := %q\n", cleanPath(path))
	fmt.Fprintln(f, "uri := resolveRelative(c.server, path)")

	if o.RequestBody != nil {
		for mt := range o.RequestBody.Value.Content {
			if mt != "application/json" {
				break
			}

			// We need to encode the request body as json.
			fmt.Fprintln(f, "// Encode the request body as json.")
			fmt.Fprintln(f, "b := new(bytes.Buffer)")
			fmt.Fprintln(f, "if err := json.NewEncoder(b).Encode(j); err != nil {")
			if respType != "" {
				fmt.Fprintln(f, `return nil, fmt.Errorf("encoding json body request failed: %v", err)`)
			} else {
				fmt.Fprintln(f, `return fmt.Errorf("encoding json body request failed: %v", err)`)
			}
			fmt.Fprintln(f, "}")
			reqBodyParam = "b"
			break
		}

	}

	// Create the request.
	fmt.Fprintln(f, "// Create the request.")

	fmt.Fprintf(f, "req, err := http.NewRequest(%q, uri, %s)\n", method, reqBodyParam)
	fmt.Fprintln(f, "if err != nil {")
	if respType != "" {
		fmt.Fprintln(f, `return nil, fmt.Errorf("error creating request: %v", err)`)
	} else {
		fmt.Fprintln(f, `return fmt.Errorf("error creating request: %v", err)`)
	}
	fmt.Fprintln(f, "}")

	// Add the parameters to the url.
	if len(params) > 0 {
		fmt.Fprintln(f, "// Add the parameters to the url.")
		fmt.Fprintln(f, "if err := expandURL(req.URL, map[string]string{")
		for name, p := range params {
			t := printType(name, p.Schema)
			if t == "string" {
				fmt.Fprintf(f, "	%q: %s,\n", strcase.ToLowerCamel(name), strcase.ToLowerCamel(name))
			} else if t == "int" {
				fmt.Fprintf(f, "	%q: strconv.Itoa(%s),\n", strcase.ToLowerCamel(name), strcase.ToLowerCamel(name))
			} else {
				fmt.Fprintf(f, "	%q: string(%s),\n", strcase.ToLowerCamel(name), strcase.ToLowerCamel(name))
			}
		}
		fmt.Fprintln(f, "}); err != nil {")
		if respType != "" {
			fmt.Fprintln(f, `return nil, fmt.Errorf("expanding URL with parameters failed: %v", err)`)
		} else {
			fmt.Fprintln(f, `return fmt.Errorf("expanding URL with parameters failed: %v", err)`)
		}
		fmt.Fprintln(f, "}")
	}

	// Send the request.
	fmt.Fprintln(f, "// Send the request.")
	fmt.Fprintln(f, "resp, err := c.client.Do(req)")
	fmt.Fprintln(f, "if err != nil {")
	if respType != "" {
		fmt.Fprintln(f, `return nil, fmt.Errorf("error sending request: %v", err)`)
	} else {
		fmt.Fprintln(f, `return fmt.Errorf("error sending request: %v", err)`)
	}
	fmt.Fprintln(f, "}")
	fmt.Fprintln(f, "defer resp.Body.Close()")

	// Check the response if there were any errors.
	fmt.Fprintln(f, "// Check the response.")
	fmt.Fprintln(f, "if err := checkResponse(resp); err != nil {")
	if respType != "" {
		fmt.Fprintln(f, "return nil, err")
	} else {
		fmt.Fprintln(f, "return err")
	}
	fmt.Fprintln(f, "}")

	if respType != "" {
		// Decode the body from the response.
		fmt.Fprintln(f, "// Decode the body from the response.")
		fmt.Fprintln(f, "if resp.Body == nil {")
		fmt.Fprintln(f, `return nil, errors.New("request returned an empty body in the response")`)
		fmt.Fprintln(f, "}")

		fmt.Fprintf(f, "var body %s\n", respType)
		fmt.Fprintln(f, "if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {")
		fmt.Fprintln(f, `return nil, fmt.Errorf("error decoding response body: %v", err)`)
		fmt.Fprintln(f, "}")

		// Return the response.
		fmt.Fprintln(f, "// Return the response.")
		fmt.Fprintln(f, "return &body, nil")
	} else {
		fmt.Fprintln(f, "// Return.")
		fmt.Fprintln(f, "return nil")
	}

	// Close the method.
	fmt.Fprintln(f, "}")
	fmt.Fprintln(f, "")
}

// cleanPath returns the path as a function we can use for a go template.
func cleanPath(path string) string {
	path = strings.Replace(path, "{", "{{.", -1)
	return strings.Replace(path, "}", "}}", -1)
}

func getSuccessResponseType(o *openapi3.Operation) string {
	for name, response := range o.Responses {
		statusCode, err := strconv.Atoi(name)
		if err != nil {
			fmt.Printf("error converting %q to an integer: %v\n", name, err)
			os.Exit(1)
		}

		if statusCode < 200 || statusCode >= 300 {
			// Continue early, we just want the successful response.
			continue
		}

		if response.Ref != "" {
			fmt.Printf("[WARN] TODO: skipping response for %q, since it is a reference: %q\n", name, response.Ref)
			continue
		}

		for _, content := range response.Value.Content {
			if content.Schema.Ref != "" {
				return getReferenceSchema(content.Schema)
			}

			return fmt.Sprintf("%sResponse", strcase.ToCamel(o.OperationID))
		}
	}

	return ""
}

// writeSchemaType writes a type definition for the given schema.
// The additional parameter is only used as a suffix for the type name.
// This is mostly for oneOf types.
func writeSchemaType(f *os.File, name string, s *openapi3.Schema, additionalName string) {
	otype := s.Type
	fmt.Printf("writing type for schema %q -> %s\n", name, otype)

	name = printProperty(name)
	typeName := strings.TrimSpace(fmt.Sprintf("%s%s", name, printProperty(additionalName)))

	if len(s.Enum) == 0 {
		// Write the type description.
		writeSchemaTypeDescription(typeName, s, f)
	}

	if otype == "string" {
		// If this is an enum, write the enum type.
		if len(s.Enum) > 0 {
			// Make sure we don't redeclare the enum type.
			if !contains(EnumStringTypes, makeSingular(typeName)) {
				// Write the type description.
				writeSchemaTypeDescription(makeSingular(typeName), s, f)

				// Write the enum type.
				fmt.Fprintf(f, "type %s string\n", makeSingular(typeName))

				EnumStringTypes = append(EnumStringTypes, makeSingular(typeName))
			}

			// Define the enum values.
			fmt.Fprintf(f, "const (\n")
			for _, v := range s.Enum {
				// Most likely, the enum values are strings.
				enum, ok := v.(string)
				if !ok {
					fmt.Printf("[WARN] TODO: enum value is not a string for %q -> %#v\n", name, v)
					continue
				}
				// Write the description of the constant.
				fmt.Fprintf(f, "// %s represents the %s `%q`.\n", strcase.ToCamel(fmt.Sprintf("%s_%s", makeSingular(name), enum)), makeSingular(name), enum)
				fmt.Fprintf(f, "\t%s %s = %q\n", strcase.ToCamel(fmt.Sprintf("%s_%s", makeSingular(name), enum)), makeSingular(name), enum)
			}
			// Close the enum values.
			fmt.Fprintf(f, ")\n")

			// Make the enum a collection of the values.
			// Add a description.
			// TODO: Fix this so that we can do this out of band of OneOf types.
			if !contains(EnumStringTypes, makeSingular(typeName)) {
				fmt.Fprintf(f, "// %s is the collection of all %s values.\n", makePlural(name), makeSingular(name))
				fmt.Fprintf(f, "var %s = []%s{\n", makePlural(name), makeSingular(name))
				for _, v := range s.Enum {
					// Most likely, the enum values are strings.
					enum, ok := v.(string)
					if !ok {
						fmt.Printf("[WARN] TODO: enum value is not a string for %q -> %#v\n", name, v)
						continue
					}
					fmt.Fprintf(f, "\t%s,\n", strcase.ToCamel(fmt.Sprintf("%s_%s", makeSingular(name), enum)))
				}
				// Close the enum values.
				fmt.Fprintf(f, "}\n")
			}
		} else {
			fmt.Fprintf(f, "type %s string\n", name)
		}
	} else if otype == "integer" {
		fmt.Fprintf(f, "type %s int\n", name)
	} else if otype == "number" {
		fmt.Fprintf(f, "type %s float64\n", name)
	} else if otype == "boolean" {
		fmt.Fprintf(f, "type %s bool\n", name)
	} else if otype == "array" {
		fmt.Fprintf(f, "type %s []%s\n", name, s.Items.Value.Type)
	} else if otype == "object" {
		recursive := false
		fmt.Fprintf(f, "type %s struct {\n", typeName)
		// We want to ensure we keep the order so the diffs don't look like shit.
		keys := make([]string, 0)
		for k := range s.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := s.Properties[k]
			// Check if we need to generate a type for this type.
			typeName := printType(k, v)

			if isLocalEnum(v) {
				recursive = true
				typeName = fmt.Sprintf("%s%s", name, printProperty(k))
			}

			if isLocalObject(v) {
				recursive = true
				fmt.Printf("[WARN] TODO: skipping object for %q -> %#v\n", name, v)
				typeName = fmt.Sprintf("%s%s", name, printProperty(k))
			}

			if v.Value.Description != "" {
				fmt.Fprintf(f, "\t// %s is %s\n", printProperty(k), toLowerFirstLetter(strings.ReplaceAll(v.Value.Description, "\n", "\n// ")))
			}
			fmt.Fprintf(f, "\t%s %s `json:\"%s,omitempty\" yaml:\"%s,omitempty\"`\n", printProperty(k), typeName, k, k)
		}
		fmt.Fprintf(f, "}\n")

		if recursive {
			// Add a newline at the end of the type.
			fmt.Fprintln(f, "")

			// Iterate over the properties and write the types, if we need to.
			for k, v := range s.Properties {
				if isLocalEnum(v) {
					writeSchemaType(f, fmt.Sprintf("%s%s", name, printProperty(k)), v.Value, "")
				}

				if isLocalObject(v) {
					writeSchemaType(f, fmt.Sprintf("%s%s", name, printProperty(k)), v.Value, "")
				}
			}
		}
	} else {
		if s.OneOf != nil {
			// We want to convert these to a different data type to be more idiomatic.
			// But first, we need to make sure we have a type for each one.
			var oneOfTypes []string
			for _, v := range s.OneOf {
				// We want to iterate over the properties of the embedded object
				// and find the type that is a string.
				var typeName string
				for prop, p := range v.Value.Properties {
					if p.Value.Type == "string" {
						fmt.Printf("[WARN] TODO: oneOf for %q -> %q %#v\n", name, prop, p.Value)
						if p.Value.Enum != nil {
							// We want to get the enum value.
							// Make sure there is only one.
							if len(p.Value.Enum) != 1 {
								fmt.Printf("[WARN] TODO: oneOf for %q -> %q enum %#v\n", name, prop, p.Value.Enum)
								continue
							}

							typeName = printProperty(p.Value.Enum[0].(string))
							break
						}
					}
				}
				// Basically all of these will have one type embedded in them that is a
				// string and the type, since these come from a Rust sum type.
				oneOfType := fmt.Sprintf("%s%s", name, typeName)
				writeSchemaType(f, name, v.Value, typeName)
				// Add it to our array.
				oneOfTypes = append(oneOfTypes, oneOfType)
			}

			// Okay so now we have all the oneOf types, we can write the type we will actually use.
		} else if s.AnyOf != nil {
			fmt.Printf("[WARN] TODO: skipping type for %q, since it is a ANYOF\n", name)
		} else if s.AllOf != nil {
			fmt.Printf("[WARN] TODO: skipping type for %q, since it is a ALLOF\n", name)
		}
	}

	// Add a newline at the end of the type.
	fmt.Fprintln(f, "")
}

func isLocalEnum(v *openapi3.SchemaRef) bool {
	return v.Ref == "" && v.Value.Type == "string" && len(v.Value.Enum) > 0
}

func isLocalObject(v *openapi3.SchemaRef) bool {
	return v.Ref == "" && v.Value.Type == "object" && len(v.Value.Properties) > 0
}

// formatStringType converts a string schema to a valid Go type.
func formatStringType(t *openapi3.Schema) string {
	if t.Format == "date-time" {
		return "*time.Time"
	} else if t.Format == "date" {
		return "*time.Time"
	} else if t.Format == "time" {
		return "*time.Time"
	} else if t.Format == "email" {
		return "string"
	} else if t.Format == "hostname" {
		return "string"
	} else if t.Format == "ipv4" {
		return "string"
	} else if t.Format == "ipv6" {
		return "string"
	} else if t.Format == "uri" {
		return "string"
	} else if t.Format == "uuid" {
		return "string"
	} else if t.Format == "uuid3" {
		return "string"
	}

	return "string"
}

// toLowerFirstLetter returns the given string with the first letter converted to lower case.
func toLowerFirstLetter(str string) string {
	for i, v := range str {
		return string(unicode.ToLower(v)) + str[i+1:]
	}
	return ""
}

// makeSingular returns the given string but singular.
func makeSingular(s string) string {
	if strings.HasSuffix(s, "Status") {
		return s
	}
	return strings.TrimSuffix(s, "s")
}

// makePlural returns the given string but plural.
func makePlural(s string) string {
	singular := makeSingular(s)
	if strings.HasSuffix(singular, "s") {
		return singular + "es"
	}

	return singular + "s"
}

// writeSchemaTypeDescription writes the description of the given type.
func writeSchemaTypeDescription(name string, s *openapi3.Schema, f *os.File) {
	if s.Description != "" {
		fmt.Fprintf(f, "// %s is %s\n", name, toLowerFirstLetter(strings.ReplaceAll(s.Description, "\n", "\n// ")))
	} else {
		fmt.Fprintf(f, "// %s is the type definition for a %s.\n", name, name)
	}
}

// writeReponseTypeDescription writes the description of the given type.
func writeResponseTypeDescription(name string, r *openapi3.Response, f *os.File) {
	if r.Description != nil {
		fmt.Fprintf(f, "// %s is the response given when %s\n", name, toLowerFirstLetter(
			strings.ReplaceAll(*r.Description, "\n", "\n// ")))
	} else {
		fmt.Fprintf(f, "// %s is the type definition for a %s response.\n", name, name)
	}
}

func getReferenceSchema(v *openapi3.SchemaRef) string {
	if v.Ref != "" {
		ref := strings.TrimPrefix(v.Ref, "#/components/schemas/")
		if len(v.Value.Enum) > 0 {
			return printProperty(makeSingular(ref))
		}

		return printProperty(ref)
	}

	return ""
}

// writeResponseType writes a type definition for the given response.
func writeResponseType(f *os.File, name string, r *openapi3.Response) {
	// Write the type definition.
	for k, v := range r.Content {
		fmt.Printf("writing type for response %q -> `%s`\n", name, k)

		name := fmt.Sprintf("%sResponse", name)

		// Write the type description.
		writeResponseTypeDescription(name, r, f)

		// Print the type definition.
		s := v.Schema
		if s.Ref != "" {
			fmt.Fprintf(f, "type %s %s\n", name, getReferenceSchema(s))
			continue
		}

		writeSchemaType(f, name, s.Value, "")
	}
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}
