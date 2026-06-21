package cmd

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// prompter reads interactive answers line by line. It works the same whether
// stdin is a terminal or piped (useful for scripting and tests).
type prompter struct {
	in  *bufio.Reader
	out io.Writer
}

func newPrompter(in io.Reader, out io.Writer) *prompter {
	return &prompter{in: bufio.NewReader(in), out: out}
}

func (p *prompter) readLine() (string, error) {
	line, err := p.in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// ask prompts for a free-form value. Empty input returns def; when def is
// empty and required is true, it re-prompts.
func (p *prompter) ask(label, def string, required bool) (string, error) {
	for {
		if def != "" {
			fmt.Fprintf(p.out, "%s [%s]: ", label, def)
		} else {
			fmt.Fprintf(p.out, "%s: ", label)
		}
		v, err := p.readLine()
		if err != nil {
			return "", err
		}
		if v == "" {
			v = def
		}
		if v != "" || !required {
			return v, nil
		}
		fmt.Fprintln(p.out, "  a value is required")
	}
}

// askChoice prompts until the answer is one of options.
func (p *prompter) askChoice(label string, options []string, def string) (string, error) {
	for {
		v, err := p.ask(fmt.Sprintf("%s (%s)", label, strings.Join(options, "/")), def, true)
		if err != nil {
			return "", err
		}
		for _, o := range options {
			if v == o {
				return v, nil
			}
		}
		fmt.Fprintf(p.out, "  choose one of: %s\n", strings.Join(options, ", "))
	}
}

func (p *prompter) askYesNo(label string, def bool) (bool, error) {
	d := "y/N"
	if def {
		d = "Y/n"
	}
	for {
		fmt.Fprintf(p.out, "%s (%s): ", label, d)
		v, err := p.readLine()
		if err != nil {
			return false, err
		}
		switch strings.ToLower(v) {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		fmt.Fprintln(p.out, "  answer y or n")
	}
}

// askList collects values until the user submits an empty line.
func (p *prompter) askList(label string, validate func(string) error) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for {
		fmt.Fprintf(p.out, "%s (empty line to finish): ", label)
		v, err := p.readLine()
		if err != nil {
			return nil, err
		}
		if v == "" {
			return out, nil
		}
		if err := validate(v); err != nil {
			fmt.Fprintf(p.out, "  %v\n", err)
			continue
		}
		if seen[v] {
			fmt.Fprintf(p.out, "  %q already added\n", v)
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
}
