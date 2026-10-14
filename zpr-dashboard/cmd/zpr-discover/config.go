package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/discovery"
)

func loadConfig(path string, flags *flag.FlagSet) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if len(data) > 64<<10 {
		return errors.New("file exceeds 64 KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return errors.New("expected a JSON object")
	}
	explicit := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		name := token.(string)
		f := flags.Lookup(name)
		if f == nil || name == "config" || name == "authorized" {
			return fmt.Errorf("unsupported key %q (authorization must be supplied on the command line)", name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate key %q", name)
		}
		seen[name] = true
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("%s must not be null", name)
		}
		var value string
		if name == "targets" {
			var entries []string
			err = json.Unmarshal(raw, &entries)
			if err == nil {
				_, err = discovery.ParseTargetEntries(entries)
			}
			value = strings.Join(entries, ",")
		} else {
			switch f.Value.(flag.Getter).Get().(type) {
			case string, time.Duration:
				err = json.Unmarshal(raw, &value)
			case bool:
				var boolean bool
				err = json.Unmarshal(raw, &boolean)
				value = strconv.FormatBool(boolean)
			case int:
				var integer int
				err = json.Unmarshal(raw, &integer)
				value = strconv.Itoa(integer)
			default:
				return fmt.Errorf("unsupported option type for %s", name)
			}
		}
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if _, ok := f.Value.(flag.Getter).Get().(time.Duration); ok {
			if _, err := time.ParseDuration(value); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		if !explicit[name] {
			if err := flags.Set(name, value); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("unexpected data after config object")
	}
	return nil
}
