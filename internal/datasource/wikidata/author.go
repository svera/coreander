package wikidata

import (
	"time"

	"github.com/rickb777/date/v2"
	"github.com/rickb777/date/v2/timespan"
)

// Wikidata properties IDs
const (
	propertyInstanceOf  = "P31"
	propertyImage       = "P18"
	propertySexOrGender = "P21"
	propertyDateOfBirth = "P569"
	propertyDateOfDeath = "P570"
	propertyWebsite     = "P856"
	propertyPseudonym   = "P742"
)

// Wikidata "instance of" values
const (
	instanceOfHuman               = "Q5"
	instanceOfPseudonym           = "Q61002"
	instanceOfPenName             = "Q127843"
	instanceOfCollectivePseudonym = "Q16017119"
)

// Wikidata gender values
const (
	genderMale             = "Q6581097"
	genderFemale           = "Q6581072"
	genderIntersex         = "Q1097630"
	genderTrasgenderFemale = "Q1052281"
	genderTrasgenderMale   = "Q2449503"
)

const (
	InstanceUnknown = iota
	InstanceHuman
	InstancePseudonym
	InstancePenName
	InstanceCollectivePseudonym
)

const (
	GenderUnknown = iota
	GenderMale
	GenderFemale
	GenderIntersex
	GenderTrasgenderFemale
	GenderTrasgenderMale
)

type Authordata struct {
	wikidataEntityId string
	wikipediaLink    map[string]string
	instanceOf       int
	description      map[string]string
	dateOfBirth      date.Date
	yearOfBirth      int // Used when dateOfBirth is not available
	dateOfDeath      date.Date
	yearOfDeath      int // Used when dateOfDeath is not available
	website          string
	image            string
	retrievedOn      time.Time
	gender           int
}

func (a Authordata) Description(language string) string {
	return a.description[language]
}

func (a Authordata) DateOfBirth() date.Date {
	return a.dateOfBirth
}

func (a Authordata) DateOfDeath() date.Date {
	return a.dateOfDeath
}

func (a Authordata) Website() string {
	return a.website
}

func (a Authordata) Image() string {
	return a.image
}

func (a Authordata) InstanceOf() int {
	return a.instanceOf
}

func (a Authordata) YearOfBirth() int {
	return a.yearOfBirth
}

func (a Authordata) YearOfBirthAbs() int {
	if a.yearOfBirth < 0 {
		return -a.yearOfBirth
	}
	return a.yearOfBirth
}

func (a Authordata) YearOfDeathAbs() int {
	if a.yearOfDeath < 0 {
		return -a.yearOfDeath
	}
	return a.yearOfDeath
}

func (a Authordata) YearOfDeath() int {
	return a.yearOfDeath
}

func (a Authordata) Age() int {
	if a.dateOfBirth == 0 {
		return 0
	}

	period := timespan.BetweenDates(a.dateOfBirth, date.Today())
	if a.dateOfDeath > 0 {
		period = timespan.BetweenDates(a.dateOfBirth, a.dateOfDeath)
	}

	return int(period.Days() / 365)
}

func (a Authordata) WikipediaLink(language string) string {
	return a.wikipediaLink[language]
}

func (a Authordata) Gender() int {
	return a.gender
}

func (a Authordata) SourceID() string {
	return a.wikidataEntityId
}

func (a Authordata) RetrievedOn() time.Time {
	return a.retrievedOn
}
