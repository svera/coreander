package wikidata

const (
	claimInstanceOf  = "P31"
	claimImage       = "P18"
	claimSexOrGender = "P21"
	claimDateOfBirth = "P569"
	claimDateOfDeath = "P570"
	claimWebsite     = "P856"
)

// Wikidata instance of values
const (
	instanceOfHuman               = "Q5"
	instanceOfPseudonym           = "Q61002"
	instanceOfPenName             = "Q127843"
	instanceOfCollectivePseudonym = "Q16017119"
)

const (
	isHuman = iota
	isPseudonym
	isPenName
	isCollectivePseudonym
)

const (
	male             = "Q6581097"
	female           = "Q6581072"
	intersex         = "Q1097630"
	trasgenderFemale = "Q1052281"
	trasgenderMale   = "Q2449503"
)
