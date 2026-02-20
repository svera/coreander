package pdf_parser

import (
	"errors"
	"strings"
)

const BufferSize = 50
const BufferSize300 = 300

var (
	fileIsNotPdfError         = errors.New("file is not pdf")
	cannotReadXrefOffset      = errors.New("cannot read OriginalXrefOffset")
	cannotParseXrefOffset     = errors.New("cannot parse OriginalXrefOffset")
	cannotParseXrefSection    = errors.New("cannot parse XrefSection")
	cannotFindObjectById      = errors.New("cannot find object in Xref table")
	cannotFindRootObject      = errors.New("cannot find root object")
	cannotFindInfoObject      = errors.New("cannot find info object")
	cannotParseTrailer        = errors.New("cannot parse trailer section")
	cannotParseObject         = errors.New("cannot parse xref Object")
	unsupportedParseContent   = errors.New("unsupported stream decode content")
	cannotFindStreamContent   = errors.New("cannot find stream content")
	invalidXrefTableStructure = errors.New("invalid Xref table structure")
	invalidSearchIndex        = errors.New("invalid search index")
)

type PdfInfo struct {
	PdfVersion               string
	OriginalXrefOffset       int64
	OriginalTrailerSection   TrailerSection
	AdditionalTrailerSection []*TrailerSection
	XrefTable                []*XrefTable
	Root                     RootObject
	Info                     InfoObject
	Metadata                 Metadata
	PagesCount               int
}

func (pdf *PdfInfo) GetTitle() string {
	if pdf.Info.Title != "" {
		return pdf.Info.Title
	}

	if pdf.Metadata.RdfMeta != nil {
		return pdf.Metadata.RdfMeta.Title
	}

	return ""
}

func (pdf *PdfInfo) GetAuthor() string {
	if pdf.Info.Author != "" {
		return pdf.Info.Author
	}

	if pdf.Metadata.RdfMeta != nil {
		return pdf.Metadata.RdfMeta.Creator
	}

	return ""
}

func (pdf *PdfInfo) GetCreator() string {
	if pdf.Info.Creator != "" {
		return pdf.Info.Creator
	}
	return ""
}

func (pdf *PdfInfo) GetISBN() string {
	if pdf.Metadata.RdfMeta != nil {
		return pdf.Metadata.RdfMeta.Isbn
	}

	return ""
}

func (pdf *PdfInfo) GetPublishers() []string {
	if pdf.Metadata.RdfMeta != nil {
		return pdf.Metadata.RdfMeta.Publishers
	}

	return []string{}
}

func (pdf *PdfInfo) GetLanguages() []string {
	if pdf.Metadata.RdfMeta != nil {
		return pdf.Metadata.RdfMeta.Languages
	}

	return []string{}
}

func (pdf *PdfInfo) GetLanguage() string {
	if pdf.Metadata.RdfMeta != nil {
		return strings.Join(pdf.Metadata.RdfMeta.Languages, ",")
	}

	return ""
}

func (pdf *PdfInfo) GetDate() string {
	if pdf.Metadata.RdfMeta != nil {
		return pdf.Metadata.RdfMeta.Date
	}

	return ""
}

func (pdf *PdfInfo) GetPublisherInfo() string {
	if pdf.Metadata.RdfMeta != nil {
		return strings.Join(pdf.Metadata.RdfMeta.Publishers, ",")
	}

	return ""
}

func (pdf *PdfInfo) GetDescription() string {
	if pdf.Metadata.RdfMeta != nil {
		return pdf.Metadata.RdfMeta.Description
	}

	return ""
}

func (pdf *PdfInfo) GetPagesCount() int {
	return pdf.PagesCount
}

func (pdf *PdfInfo) GetCover(filepath string) bool {
	//TODO finish this
	isSuccess := false
	return isSuccess
}

type TrailerSection struct {
	IdRaw string
	Info  ObjectIdentifier
	Root  ObjectIdentifier
	Size  string
	Prev  int64
}

type ObjectIdentifier struct {
	ObjectNumber     int
	GenerationNumber int
	KeyWord          string
}

type ObjectSubsectionElement struct {
	Id               int
	ObjectNumber     int
	GenerationNumber int
	KeyWord          string
}

/*
	Object subsection that contain list of objects for this object
*/
type ObjectSubsection struct {
	Id                      int // objectId
	ObjectsCount            int
	FirstSubsectionObjectId int
	LastSubsectionObjectId  int
	Elements                map[int]*ObjectSubsectionElement
}

type XrefTable struct {
	Objects           map[int]*ObjectSubsectionElement
	ObjectSubsections map[int]*ObjectSubsection
	SectionStart      int64
}

type InfoObject struct {
	Title        string
	Author       string
	Creator      string
	CreationDate string
	Producer     string
	ModDate      string
}

type RootObject struct {
	Type       string
	Pages      *ObjectIdentifier
	Metadata   *ObjectIdentifier
	PageLabels *ObjectIdentifier
	Lang       string
}

type Metadata struct {
	Type          string
	Subtype       string
	Length        int64
	DL            int64
	RawStreamData []byte
	RdfMeta       *MetaDataRdf
}

type MetaDataRdf struct {
	Title       string
	Description string
	Creator     string
	Date        string
	Isbn        string

	Publishers []string
	Languages  []string
}
