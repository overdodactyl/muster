package render

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// Real-name display. Off by default; when on, LookupName / LookupNameFull
// swap the lanid for the display name from the resolver cache. The TUI
// populates the cache asynchronously via getent; the render layer is
// oblivious to how names arrive.
//
// All state here is read/written on the bubbletea event-loop goroutine, so no
// locking is needed. Same shape as SetTheme / CurrentTheme above.

var (
	namesEnabled bool
	nameCache    map[string]string
)

func SetNames(on bool) { namesEnabled = on }

func NamesEnabled() bool { return namesEnabled }

// SetNameResolver installs the lanid → display-name map. Replaces any prior
// map; the caller (TUI) is expected to have merged incremental results before
// calling.
func SetNameResolver(m map[string]string) { nameCache = m }

// LookupName returns the display value for a lanid. When names are enabled
// and a non-empty entry exists in the cache, returns the resolved name;
// otherwise returns the lanid unchanged. Safe to call with any string.
func LookupName(lanid string) string {
	if !namesEnabled || lanid == "" {
		return lanid
	}
	if name, ok := nameCache[lanid]; ok && name != "" {
		return name
	}
	return lanid
}

// LookupNameFull returns "<Full Name> (<lanid>)" when a name is resolved and
// names are enabled; otherwise falls back to the lanid. Use in detail
// overlays / cards where width is not the constraint and the identifier is
// still worth showing alongside the name.
func LookupNameFull(lanid string) string {
	if !namesEnabled || lanid == "" {
		return lanid
	}
	if name, ok := nameCache[lanid]; ok && name != "" {
		return name + " (" + lanid + ")"
	}
	return lanid
}

// PrewarmNames runs `getent passwd <lanids>` synchronously and merges the
// results into the name cache. No-op when names are disabled or the list is
// empty. Errors are swallowed — LookupName falls back to lanid on any miss,
// which is the same behavior as an unpopulated cache. Use from static CLI
// entry points; the TUI has its own async pipeline that populates the same
// cache without blocking the event loop.
func PrewarmNames(lanids []string) {
	if !namesEnabled || len(lanids) == 0 {
		return
	}
	seen := map[string]bool{}
	args := []string{"passwd"}
	for _, id := range lanids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		args = append(args, id)
	}
	if len(args) == 1 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "getent", args...).Output()
	if err != nil {
		return
	}
	if nameCache == nil {
		nameCache = map[string]string{}
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, ":", 7)
		if len(fields) < 5 {
			continue
		}
		if name := ParseGECOSName(fields[4]); name != "" {
			nameCache[fields[0]] = name
		}
	}
}

// ParseGECOSName extracts a friendly display name from a passwd GECOS field.
// Handles two common layouts:
//
//	Traditional:    "Full Name,office,phone,phone,other"       → "Full Name"
//	Institutional:  "Last, First M. (Nickname),office,phone…"  → "Nickname Last" (or "First Last")
//
// The distinguishing signal is the space after the comma: GECOS field
// separators never have one, but "Last, First" always does. When a
// parenthesised nickname is present it's preferred as the given name;
// otherwise the first whitespace-delimited word after the comma is used
// (drops middle initials cleanly). Returns "" for an empty GECOS.
func ParseGECOSName(gecos string) string {
	s := strings.TrimSpace(gecos)
	if s == "" {
		return ""
	}
	if idx := strings.Index(s, ", "); idx > 0 && idx+2 < len(s) {
		last := strings.TrimSpace(s[:idx])
		rest := s[idx+2:]
		if c := strings.Index(rest, ","); c >= 0 {
			rest = rest[:c]
		}
		if lp := strings.Index(rest, "("); lp != -1 {
			if rp := strings.Index(rest[lp:], ")"); rp > 0 {
				if nick := strings.TrimSpace(rest[lp+1 : lp+rp]); nick != "" && last != "" {
					return nick + " " + last
				}
			}
			rest = strings.TrimSpace(rest[:lp])
		}
		if first := strings.Fields(rest); len(first) > 0 && last != "" {
			return first[0] + " " + last
		}
	}
	if idx := strings.Index(s, ","); idx > 0 {
		return strings.TrimSpace(s[:idx])
	}
	// Bare GECOS with no comma. Real names are ≤4 words; anything longer is
	// almost certainly a service-account description ("To run the X cron
	// job…"). In that case return "" so the caller falls back to the lanid.
	if strings.Count(s, " ") > 3 {
		return ""
	}
	return s
}
