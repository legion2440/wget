package cli

import (
	"fmt"
	"strings"
)

type Options struct {
	URL             string
	OutputName      string
	OutputDir       string
	RateLimit       string
	InputFile       string
	Background      bool
	Mirror          bool
	Reject          []string
	Exclude         []string
	ConvertLinks    bool
	BackgroundChild bool
}

func Parse(args []string) (Options, error) {
	var opts Options
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}

		switch {
		case arg == "-B":
			opts.Background = true
		case arg == "--mirror":
			opts.Mirror = true
		case arg == "--convert-links":
			opts.ConvertLinks = true
		case arg == "--background-child":
			opts.BackgroundChild = true
		case isValueOption(arg, "-O", "--output-document"):
			value, next, err := optionValue(args, i, arg, "-O", "--output-document")
			if err != nil {
				return Options{}, err
			}
			opts.OutputName = value
			i = next
		case isValueOption(arg, "-P", "--directory-prefix"):
			value, next, err := optionValue(args, i, arg, "-P", "--directory-prefix")
			if err != nil {
				return Options{}, err
			}
			opts.OutputDir = value
			i = next
		case isValueOption(arg, "-i", "--input-file"):
			value, next, err := optionValue(args, i, arg, "-i", "--input-file")
			if err != nil {
				return Options{}, err
			}
			opts.InputFile = value
			i = next
		case isValueOption(arg, "--rate-limit", ""):
			value, next, err := optionValue(args, i, arg, "--rate-limit", "")
			if err != nil {
				return Options{}, err
			}
			opts.RateLimit = value
			i = next
		case isValueOption(arg, "-R", "--reject"):
			value, next, err := optionValue(args, i, arg, "-R", "--reject")
			if err != nil {
				return Options{}, err
			}
			opts.Reject = splitList(value)
			i = next
		case isValueOption(arg, "-X", "--exclude"):
			value, next, err := optionValue(args, i, arg, "-X", "--exclude")
			if err != nil {
				return Options{}, err
			}
			opts.Exclude = splitList(value)
			i = next
		case strings.HasPrefix(arg, "-"):
			return Options{}, fmt.Errorf("unknown option %q", arg)
		default:
			positional = append(positional, arg)
		}
	}

	if len(positional) > 1 {
		return Options{}, fmt.Errorf("expected one URL, got %d", len(positional))
	}
	if len(positional) == 1 {
		opts.URL = positional[0]
	}
	if opts.InputFile == "" && opts.URL == "" {
		return Options{}, fmt.Errorf("missing URL")
	}
	if opts.InputFile != "" && opts.URL != "" {
		return Options{}, fmt.Errorf("URL and -i/--input-file cannot be used together")
	}
	if opts.Mirror && opts.InputFile != "" {
		return Options{}, fmt.Errorf("--mirror cannot be used with -i/--input-file")
	}
	if (len(opts.Reject) > 0 || len(opts.Exclude) > 0 || opts.ConvertLinks) && !opts.Mirror {
		return Options{}, fmt.Errorf("-R/--reject, -X/--exclude and --convert-links require --mirror")
	}
	if opts.Mirror && opts.OutputName != "" {
		return Options{}, fmt.Errorf("-O/--output-document cannot be used with --mirror")
	}
	if opts.Mirror && opts.RateLimit != "" {
		return Options{}, fmt.Errorf("--rate-limit cannot be used with --mirror")
	}
	return opts, nil
}

func isValueOption(arg, short, long string) bool {
	for _, name := range []string{short, long} {
		if name == "" {
			continue
		}
		if arg == name || strings.HasPrefix(arg, name+"=") {
			return true
		}
		if strings.HasPrefix(name, "-") && !strings.HasPrefix(name, "--") && strings.HasPrefix(arg, name) && len(arg) > len(name) {
			return true
		}
	}
	return false
}

func optionValue(args []string, index int, arg, short, long string) (string, int, error) {
	for _, name := range []string{short, long} {
		if name == "" {
			continue
		}
		if arg == name {
			if index+1 >= len(args) {
				return "", index, fmt.Errorf("option %s requires a value", name)
			}
			if args[index+1] == "" {
				return "", index, fmt.Errorf("option %s requires a non-empty value", name)
			}
			return args[index+1], index + 1, nil
		}
		if strings.HasPrefix(arg, name+"=") {
			value := strings.TrimPrefix(arg, name+"=")
			if value == "" {
				return "", index, fmt.Errorf("option %s requires a non-empty value", name)
			}
			return value, index, nil
		}
		if !strings.HasPrefix(name, "--") && strings.HasPrefix(arg, name) && len(arg) > len(name) {
			value := strings.TrimPrefix(strings.TrimPrefix(arg, name), "=")
			if value == "" {
				return "", index, fmt.Errorf("option %s requires a non-empty value", name)
			}
			return value, index, nil
		}
	}
	return "", index, fmt.Errorf("invalid option %q", arg)
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
