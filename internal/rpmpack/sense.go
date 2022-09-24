package rpmpack

import (
	"fmt"
	"regexp"
	"strings"
)

type rpmSense uint32

// https://github.com/rpm-software-management/rpm/blob/ab01b5eacf9ec6a07a5d9e1991ef476a12d264fd/include/rpm/rpmds.h#L27
// SenseAny (0) specifies no specific version compare
// SenseLess (2) specifies less then the specified version
// SenseGreater (4) specifies greater then the specified version
// SenseEqual (8) specifies equal to the specified version
const (
	SenseAny  rpmSense = 0
	SenseLess          = 1 << iota
	SenseGreater
	SenseEqual
	SenseRPMLIB rpmSense = 1 << 24
)

var relationMatch = regexp.MustCompile(`([^=<>\s]*)\s*((?:=|>|<)*)\s*(.*)?`)

// Relation is the structure of rpm sense relationships
type Relation struct {
	Name    string
	Version string
	Sense   rpmSense
}

// String return the string representation of the Relation
func (r *Relation) String() string {
	return fmt.Sprintf("%s%v%s", r.Name, r.Sense, r.Version)
}

// Equal compare the equality of two relations
func (r *Relation) Equal(o *Relation) bool {
	return r.Name == o.Name && r.Version == o.Version && r.Sense == o.Sense
}

// Relations is a slice of Relation pointers
type Relations []*Relation

// String return the string representation of the Relations
func (r *Relations) String() string {
	var val []string
	for _, rel := range *r {
		val = append(val, rel.String())
	}
	return strings.Join(val, ",")
}

// Set parse a string into a Relation and append it to the Relations slice if it is missing
// this is used by the flag package
func (r *Relations) Set(value string) error {
	relation, err := NewRelation(value)
	if err != nil {
		return err
	}
	r.addIfMissing(relation)

	return nil
}

func (r *Relations) addIfMissing(value *Relation) {
	for _, relation := range *r {
		if relation.Equal(value) {
			return
		}
	}

	*r = append(*r, value)
}

// AddToIndex add the relations to the specified category on the index
func (r *Relations) AddToIndex(h *index, nameTag, versionTag, flagsTag int) error {
	var (
		num      = len(*r)
		names    = make([]string, num)
		versions = make([]string, num)
		flags    = make([]uint32, num)
	)

	if num == 0 {
		return nil
	}

	for idx, relation := range *r {
		names[idx] = relation.Name
		versions[idx] = relation.Version
		flags[idx] = uint32(relation.Sense)
	}

	h.Add(nameTag, EntryStringSlice(names))
	h.Add(versionTag, EntryStringSlice(versions))
	h.Add(flagsTag, EntryUint32(flags))

	return nil
}

// NewRelation parse a string into a Relation
func NewRelation(related string) (*Relation, error) {
	var (
		err   error
		sense rpmSense
		name,
		version string
	)

	if strings.HasPrefix(related, "(") && strings.HasSuffix(related, ")") {
		// This is a `rich` dependency which must be parsed at install time
		// https://rpm-software-management.github.io/rpm/manual/boolean_dependencies.html
		sense = SenseAny
		name = related
	} else {
		parts := relationMatch.FindStringSubmatch(related)
		if sense, err = parseSense(parts[2]); err != nil {
			return nil, err
		}
		name = parts[1]
		version = parts[3]
	}

	return &Relation{
		Name:    name,
		Version: version,
		Sense:   sense,
	}, nil
}

var stringToSense = map[string]rpmSense{
	"":   SenseAny,
	"<":  SenseLess,
	">":  SenseGreater,
	"=":  SenseEqual,
	"<=": SenseLess | SenseEqual,
	">=": SenseGreater | SenseEqual,
}

// String return the string representation of the rpmSense
func (r rpmSense) String() string {
	var (
		val rpmSense
		ret string
	)

	for ret, val = range stringToSense {
		if r == val {
			return ret
		}
	}

	return "unknown"
}

func parseSense(sense string) (rpmSense, error) {
	var (
		ret rpmSense
		ok  bool
	)
	if ret, ok = stringToSense[sense]; !ok {
		return SenseAny, fmt.Errorf("unknown sense value: %s", sense)
	}

	return ret, nil
}
