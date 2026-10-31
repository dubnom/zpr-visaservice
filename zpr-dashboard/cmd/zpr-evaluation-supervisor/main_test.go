package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestControl(t *testing.T) {
	for _, action := range []string{"start", "stop"} {
		t.Run(action, func(t *testing.T) {
			var calls [][]string
			run := func(_ context.Context, args ...string) (string, error) {
				calls = append(calls, args)
				if action == "start" {
					return "active\n", nil
				}
				return "inactive\n", nil
			}
			for range 2 {
				if err := control(context.Background(), []string{"zpr-evaluation-northstar.service", action}, run); err != nil {
					t.Fatal(err)
				}
			}
			want := [][]string{
				{"--user", action, "zpr-evaluation-northstar.service"},
				{"--user", "show", "--property=ActiveState", "--value", "zpr-evaluation-northstar.service"},
			}
			want = append(want, want...)
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("calls=%v", calls)
			}
		})
	}
}

func TestControlRejectsInvalidInput(t *testing.T) {
	for _, args := range [][]string{
		nil, {"zpr-evaluation-northstar.service"}, {"--all", "stop"},
		{"ssh.service", "stop"}, {"zpr-evaluation-../northstar.service", "start"},
		{"zpr-evaluation-northstar.service;echo", "start"},
		{"zpr-evaluation-northstar-.service", "start"},
		{"zpr-evaluation-northstar.service", "restart"},
		{"zpr-evaluation-northstar.service", "stop", "other.service"},
	} {
		err := control(context.Background(), args, func(context.Context, ...string) (string, error) {
			t.Fatal("invalid input reached systemctl")
			return "", nil
		})
		if err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestControlFailures(t *testing.T) {
	for _, action := range []string{"start", "stop"} {
		for _, failure := range []string{"command", "readback", "failed", "activating", "deactivating", "empty"} {
			t.Run(action+"/"+failure, func(t *testing.T) {
				calls := 0
				err := control(context.Background(), []string{"zpr-evaluation-northstar.service", action}, func(context.Context, ...string) (string, error) {
					calls++
					if failure == "command" || failure == "readback" && calls == 2 {
						return "", errors.New("systemctl unavailable")
					}
					if failure == "empty" {
						return "", nil
					}
					return failure, nil
				})
				if err == nil {
					t.Fatal("failure reported as success")
				}
				if failure == "command" && action == "start" && calls != 1 {
					t.Fatal("readback after failed command")
				}
			})
		}
	}
}

func TestControlCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := control(ctx, []string{"zpr-evaluation-northstar.service", "start"}, func(ctx context.Context, _ ...string) (string, error) { return "", ctx.Err() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestControlUnloadedStop(t *testing.T) {
	for _, state := range []string{
		"LoadState=not-found\nActiveState=inactive\n",
		"ActiveState=inactive\nLoadState=not-found\n",
		"LoadState=loaded\nActiveState=inactive\n",
		"LoadState=not-found\nActiveState=active\n",
		"LoadState=not-found\n",
		"ActiveState=inactive\n",
		"LoadState=not-found\nActiveState=inactive\nActiveState=active\n",
		"",
	} {
		calls := 0
		err := control(context.Background(), []string{"zpr-evaluation-northstar.service", "stop"}, func(_ context.Context, args ...string) (string, error) {
			calls++
			if calls == 1 {
				return "", errors.New("unit not loaded")
			}
			want := []string{"--user", "show", "--property=LoadState", "--property=ActiveState", "zpr-evaluation-northstar.service"}
			if !reflect.DeepEqual(args, want) {
				t.Fatalf("unloaded readback args: %v", args)
			}
			return state, nil
		})
		valid := state == "LoadState=not-found\nActiveState=inactive\n" || state == "ActiveState=inactive\nLoadState=not-found\n"
		if (err == nil) != valid {
			t.Fatalf("unloaded state %q: %v", state, err)
		}
	}
}
