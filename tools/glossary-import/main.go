package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/dop251/goja"
)

func main() {
	source, err := os.ReadFile("../../lite/data/glossary.js")
	if err != nil {
		fatal(err)
	}
	vm := goja.New()
	if _, err := vm.RunString("var window = {};"); err != nil {
		fatal(err)
	}
	if _, err := vm.RunString(string(source)); err != nil {
		fatal(err)
	}
	value, err := vm.RunString("JSON.stringify(window.GLOSSARY_DATA)")
	if err != nil {
		fatal(err)
	}
	var data any
	if err := json.Unmarshal([]byte(value.String()), &data); err != nil {
		fatal(err)
	}
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fatal(err)
	}
	out = append(out, '\n')
	if err := os.WriteFile("../../data/glossary.json", out, 0o644); err != nil {
		fatal(err)
	}
	fmt.Println("wrote data/glossary.json")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
