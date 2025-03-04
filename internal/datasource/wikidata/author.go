package wikidata

import (
	"time"

	"github.com/svera/coreander/v4/internal/precisiondate"
)

// Wikidata properties IDs
const (
	propertyInstanceOf           = "P31"
	propertyImage                = "P18"
	propertySexOrGender          = "P21"
	propertyDateOfBirth          = "P569"
	propertyDateOfDeath          = "P570"
	propertyWebsite              = "P856"
	propertyPseudonym            = "P742"
	propertyBirthName            = "P1477"
	propertyNameInNativeLanguage = "P1559"
	propertyOfficialName         = "P1448"
)

// Wikidata "instance of" values
const (
	qidInstanceOfHuman               = "Q5"
	qidInstanceOfPseudonym           = "Q61002"
	qidInstanceOfPenName             = "Q127843"
	qidInstanceOfCollectivePseudonym = "Q16017119"
)

// Wikidata gender values
const (
	qidGenderMale             = "Q6581097"
	qidGenderFemale           = "Q6581072"
	qidGenderIntersex         = "Q1097630"
	qidGenderTrasgenderFemale = "Q1052281"
	qidGenderTrasgenderMale   = "Q2449503"
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

type Author struct {
	birthName        string
	wikidataEntityId string
	wikipediaLink    map[string]string
	instanceOf       int
	description      map[string]string
	dateOfBirth      precisiondate.PrecisionDate
	dateOfDeath      precisiondate.PrecisionDate
	website          string
	image            string
	retrievedOn      time.Time
	gender           int
	pseudonyms       []string
}

func (a Author) BirthName() string {
	return a.birthName
}

func (a Author) Description(language string) string {
	return a.description[language]
}

func (a Author) DateOfBirth() precisiondate.PrecisionDate {
	return a.dateOfBirth
}

func (a Author) DateOfDeath() precisiondate.PrecisionDate {
	return a.dateOfDeath
}

func (a Author) Website() string {
	return a.website
}

func (a Author) Image() string {
	return a.image
}

func (a Author) InstanceOf() int {
	return a.instanceOf
}

func (a Author) WikipediaLink(language string) string {
	return a.wikipediaLink[language]
}

func (a Author) Gender() int {
	return a.gender
}

func (a Author) SourceID() string {
	return a.wikidataEntityId
}

func (a Author) RetrievedOn() time.Time {
	return a.retrievedOn
}

func (a Author) Pseudonyms() []string {
	return a.pseudonyms
}
