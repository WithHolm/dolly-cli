package repo

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// The pattern language of a profile. It makes ids, phone numbers, and the values of extension attributes. It has no code and no regular
// expression, so a profile from outside cannot make more than text.
//
//	#     a run of # is a number with that many digits, with zeros in front (### is 000 to 999). Each run is one draw of the random numbers.
//	{a-b} a number from a to b. It has the width of a when a starts with 0: {01-12} is 01 to 12, {100-119} is 100 to 119.
//	{cc}  the calling code of the office of the user, without the +
//
// Everything else is text. A pattern is up to 100 characters.

var (
	domainRe = regexp.MustCompile(`^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,}$`)
	digitsRe = regexp.MustCompile(`^[0-9]{1,4}$`)
	rangeRe  = regexp.MustCompile(`^\{([0-9]{1,9})-([0-9]{1,9})\}`)
)

const (
	patternFree = iota // any of the parts
	patternSeq         // exactly one run of #, the number of the user
	maxPattern  = 100
)

// checkPattern says whether a pattern is one that the generator can use.
func checkPattern(p string, mode int) error {
	if p == "" {
		return errors.New("is needed")
	}
	if len(p) > maxPattern {
		return fmt.Errorf("is longer than %d characters", maxPattern)
	}
	runs := 0
	for i := 0; i < len(p); {
		c := p[i]
		switch {
		case c == '#':
			j := i
			for j < len(p) && p[j] == '#' {
				j++
			}
			if j-i > 9 {
				return errors.New("has a run of more than 9 #")
			}
			runs++
			i = j
		case c == '{':
			if mode == patternSeq {
				return errors.New("can have # only")
			}
			if strings.HasPrefix(p[i:], "{cc}") {
				i += 4
				continue
			}
			m := rangeRe.FindStringSubmatch(p[i:])
			if m == nil {
				return fmt.Errorf("has a { that is not {cc} or a range such as {100-119} (at %d)", i+1)
			}
			a, _ := strconv.Atoi(m[1])
			b, _ := strconv.Atoi(m[2])
			if a > b || b-a >= 1_000_000 {
				return fmt.Errorf("has the range %s, which is not from a small number to a larger one", m[0])
			}
			i += len(m[0])
		case c == '}':
			return fmt.Errorf("has a } with no { (at %d)", i+1)
		case c < 0x20 || c == 0x7f:
			return errors.New("has a control character")
		default:
			i++
		}
	}
	if mode == patternSeq && runs != 1 {
		return errors.New("needs exactly one run of #, for the number")
	}
	return nil
}

func pow10(n int) int {
	r := 1
	for i := 0; i < n; i++ {
		r *= 10
	}
	return r
}

// pattern makes a text from a pattern that checkPattern accepted. vars gives {cc}. Every # run and every range is one draw.
func (g *gen) pattern(p string, vars map[string]string) string {
	var out strings.Builder
	for i := 0; i < len(p); {
		switch c := p[i]; {
		case c == '#':
			j := i
			for j < len(p) && p[j] == '#' {
				j++
			}
			k := j - i
			fmt.Fprintf(&out, "%0*d", k, g.rng.IntN(pow10(k)))
			i = j
		case c == '{' && strings.HasPrefix(p[i:], "{cc}"):
			out.WriteString(vars["cc"])
			i += 4
		case c == '{':
			m := rangeRe.FindStringSubmatch(p[i:])
			a, _ := strconv.Atoi(m[1])
			b, _ := strconv.Atoi(m[2])
			width := 0
			if len(m[1]) > 1 && m[1][0] == '0' {
				width = len(m[1])
			}
			fmt.Fprintf(&out, "%0*d", width, a+g.rng.IntN(b-a+1))
			i += len(m[0])
		default:
			out.WriteByte(c)
			i++
		}
	}
	return out.String()
}

// numbered makes a text from a pattern with one run of #, which is the number n: E##### and 10001 is E10001. It draws nothing.
func numbered(p string, n int) string {
	i := strings.IndexByte(p, '#')
	j := i
	for j < len(p) && p[j] == '#' {
		j++
	}
	return p[:i] + fmt.Sprintf("%0*d", j-i, n) + p[j:]
}

// ---- the names of the admin accounts ----

var adminTokenRe = regexp.MustCompile(`\{[A-Za-z]+\}`)

var adminTokens = map[string]bool{"{first}": true, "{last}": true, "{f}": true, "{l}": true, "{First}": true, "{Last}": true}

// checkAdminPattern checks the pattern of an admin account. A user principal name (upn) has letters, digits, and . - _ and needs a name in it.
func checkAdminPattern(p string, upn bool) error {
	if p == "" {
		return errors.New("is needed")
	}
	if len(p) > maxText {
		return fmt.Errorf("is longer than %d characters", maxText)
	}
	tokens := adminTokenRe.FindAllString(p, -1)
	for _, t := range tokens {
		if !adminTokens[t] {
			return fmt.Errorf("has %s, and the pattern can have {first}, {last}, {f}, {l}, {First}, and {Last}", t)
		}
	}
	rest := adminTokenRe.ReplaceAllString(p, "")
	if strings.ContainsAny(rest, "{}\x00\r\n") {
		return errors.New("has a { or } that is not a name")
	}
	if upn {
		if len(tokens) == 0 {
			return errors.New("needs {first} or {last}, so that the name is the name of the owner")
		}
		for _, r := range rest {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_') {
				return fmt.Errorf("can have letters, digits, and . - _ besides the names (not %q)", r)
			}
		}
	}
	return nil
}

// adminName fills the pattern of an admin account with the name of the owner. The names in lowercase ({first}) have no accents, because a
// user principal name has none.
func adminName(p, first, last string) string {
	lf, ll := ascii(strings.ToLower(first)), ascii(strings.ToLower(last))
	initial := func(s string) string {
		for _, r := range s {
			return string(r)
		}
		return ""
	}
	return strings.NewReplacer("{first}", lf, "{last}", ll, "{f}", initial(lf), "{l}", initial(ll), "{First}", first, "{Last}", last).Replace(p)
}

// ascii takes the accents off a name, for a mail nickname and a user principal name: Håkon is hakon, and Bjørn is bjorn. What is not a
// letter or a digit is left out.
func ascii(s string) string {
	var out strings.Builder
	for _, r := range s {
		if folded, ok := asciiFold[r]; ok {
			out.WriteString(folded)
		} else if r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_') {
			out.WriteRune(r)
		}
	}
	return out.String()
}

var asciiFold = func() map[rune]string {
	m := map[rune]string{}
	for letters, to := range map[string]string{
		"àáâãäåāăą": "a", "ÀÁÂÃÄÅĀĂĄ": "A", "æ": "ae", "Æ": "AE", "çćč": "c", "ÇĆČ": "C", "ďđð": "d", "ĎĐÐ": "D", "èéêëēęě": "e",
		"ÈÉÊËĒĘĚ": "E", "ğ": "g", "Ğ": "G", "ìíîïī": "i", "ÌÍÎÏĪ": "I", "ł": "l", "Ł": "L", "ñńň": "n", "ÑŃŇ": "N", "òóôõöøō": "o",
		"ÒÓÔÕÖØŌ": "O", "œ": "oe", "Œ": "OE", "řŕ": "r", "ŘŔ": "R", "šśş": "s", "ŠŚŞ": "S", "ß": "ss", "ť": "t", "Ť": "T", "þ": "th",
		"Þ": "Th", "ùúûüūů": "u", "ÙÚÛÜŪŮ": "U", "ýÿ": "y", "ÝŸ": "Y", "žźż": "z", "ŽŹŻ": "Z",
	} {
		for _, r := range letters {
			m[r] = to
		}
	}
	return m
}()
