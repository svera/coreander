package epub

import (
	"encoding/xml"
	"io"
)

// PackageDocument carries meta information about the Rendition, provides a manifest
// of resources and defines the default reading order.
// PackageDocument is an implementation of a Package Document that intend to meet
// specification from https://www.w3.org/publishing/epub32/epub-packages.html.
// Known differences mainly aim at allowing reading information from OPF2-based
// epub.
type PackageDocument struct {
	XMLName xml.Name `xml:"http://www.idpf.org/2007/opf package"`

	// Dir attribute specifies the base text direction of the content and
	// attribute values of the carrying element and its descendants.
	// Allowed values are ltr (left-to-right) and rtl (right-to-left).
	Dir string `xml:"dir,attr,omitempty" json:",omitempty"`
	// ID attributes provides the ID [XML] of the element, which MUST be
	// unique within the document scope.
	ID string `xml:"id,attr,omitempty" json:",omitempty"`
	// Prefix attribute provides a declaration mechanism for prefixes not
	// reserved by this specification.
	Prefix string `xml:"prefix,attr,omitempty" json:",omitempty"`
	// Lang specifies the language used in the contents and attribute
	// values of the carrying element and its descendants.
	Lang string `xml:"xml:lang,attr,omitempty" json:",omitempty"`
	// UniqueIdentifier attribute takes an IDREF [XML] that identifies
	// the dc:identifier element that provides the preferred, or primary,
	// identifier.
	UniqueIdentifier string `xml:"unique-identifier,attr"`
	// The version attribute specifies the EPUB specification version to
	// which the given EPUB Package conforms.
	Version string `xml:"version,attr"`

	Metadata   *Metadata   `xml:"metadata"`
	Manifest   *Manifest   `xml:"manifest"`
	Spine      *Spine      `xml:"spine"`
	Collection *Collection `xml:"collection,omitempty" json:",omitempty"`
}

// Metadata encapsulates metadata information for the given Rendition.
type Metadata struct {
	// Identifier contains an identifier associated with the given
	// Rendition, such as a UUID, DOI or ISBN.
	Identifier []IdentifierElt `xml:"identifier"`
	// Title represents an instance of a name given to the EPUB
	// Publication.
	Title []Element `xml:"title"`
	// Language element specifies the language of the content of the
	// given Rendition.
	Language []Element `xml:"language"`

	// Contributor represents the name of a person, organization, etc.
	// that played a secondary role in the creation of the content of an
	// EPUB Publication.
	Contributor []AuthorElt `xml:"contributor,omitempty"`
	// Coverage gives the extent or scope of the publication’s content.
	Coverage []Element `xml:"coverage,omitempty"`
	// Creator represents the name of a person, organization, etc.
	// responsible for the creation of the content of the Rendition.
	Creator []AuthorElt `xml:"creator,omitempty"`
	// Date is only used to define the publication date of the EPUB
	// Publication.
	Date []DateElt `xml:"date,omitempty"`
	// Description provides a description of the publication's content.
	Description []Element `xml:"description,omitempty"`
	// Format identifies the media type or dimensions of the resource.
	Format []Element `xml:"format,omitempty"`
	// Publisher identifies the publication's publisher.
	Publisher []Element `xml:"publisher,omitempty"`
	// Relation is an identifier of an auxiliary resource and its
	// relationship to the publication.
	Relation []Element `xml:"relation,omitempty"`
	// Rights provides a statement about rights, or a reference to one.
	Rights []Element `xml:"rights,omitempty"`
	// Sources provides information regarding a prior resource from which
	// the publication was derived.
	Source []Element `xml:"source,omitempty"`
	// Subject identifies the subject of the EPUB Publication.
	Subject []Element `xml:"subject,omitempty"`
	// Type is used to indicate that the given EPUB Publication is of a
	// specialized type.
	Type []Element `xml:"type,omitempty"`

	// Meta element provides a generic means of including package
	// metadata.
	Meta []MetaLegacy `xml:"meta,omitempty"`
	// Link element is used to associate resources with the given
	// Rendition, such as metadata records.
	Link []Link `xml:"link,omitempty"`
}

//TODO: create an 'Element' type that filters Lang for Metadata attributes that
//are not supposed to have one.

// Element is a generic Metadata element.
type Element struct {
	// Dir attribute specifies the base text direction of the content and
	// attribute values of the carrying element and its descendants.
	// Allowed values are ltr (left-to-right) and rtl (right-to-left).
	Dir string `xml:"dir,attr,omitempty"`
	// ID attributes porvides the ID [XML] of the element, which MUST be
	// unique within the document scope.
	ID string `xml:"id,attr,omitempty"`
	// Lang specifies the language used in the contents and attribute
	// values of the carrying element and its descendants, as defined in
	// section 2.12 Language Identification of [XML].
	Lang string `xml:"xml:lang,attr,omitempty"`

	// Value is the Element's value
	Value string `xml:",chardata"`
}

// IdentifierElt is a specific Element that provides a string or number used to
// uniquely identify the resource.
// It 'extends' EPUB3 Element to capture possible 'opf:scheme' attribute that
// can be found in older EPUB version.
type IdentifierElt struct {
	*Element

	// Scheme attribute names the system or authority that generated or
	// assigned the text contained within the identifier element, for example
	// "ISBN" or "DOI
	Scheme string `xml:"scheme,attr,omitempty"`
}

// AuthorElt is a specific Element that provides information about a creator or
// contributor.
// It 'extends' EPUB3 Element to capture possible 'opf:role' and 'opf:file-as'
// attributes that can be found in older EPUB version.
type AuthorElt struct {
	*Element

	// FileAs attribute is used to specify a normalized form of the contents,
	// suitable for machine processing.
	FileAs string `xml:"file-as,attr,omitempty"`
	// Role attribute is used to refine the Author role. It's usually a
	// 3-character registered MARC value (http://www.loc.gov/marc/relators/).
	Role string `xml:"role,attr,omitempty"`
}

// DateElt is a specific Element that provides information about the date of
// publication.
type DateElt struct {
	*Element

	// Event attribute further detailed the event to which the date correspond
	// to.
	Event string `xml:"event,attr,omitempty"`
}

// Meta element provides a generic means of including package metadata.
type Meta struct {
	// Dir attribute specifies the base text direction of the content and
	// attribute values of the carrying element and its descendants.
	// Allowed values are ltr (left-to-right) and rtl (right-to-left).
	Dir string `xml:"dir,attr,omitempty"`
	// ID attributes provides the ID [XML] of the element, which MUST be
	// unique within the document scope.
	ID string `xml:"id,attr,omitempty"`
	// Property takes a property data type value that defines the
	// statement being made in the expression, and the text content of
	// the element represents the assertion.
	Property string `xml:"property,attr"`
	// Refines identifies the expression or resource augmented by the
	// element.
	Refines string `xml:"refines,attr,omitempty"`
	// Scheme attribute identifies the system or scheme that the
	// element's value is drawn from.
	Scheme string `xml:"scheme,attr,omitempty"`
	// Lang specifies the language used in the contents and attribute
	// values of the carrying element and its descendants.
	Lang string `xml:"xml:lang,attr,omitempty"`

	// Value is the Element's value
	Value string `xml:",chardata"`
}

// MetaLegacy extends Meta to adapt to a possible OPF2 meta statement.
type MetaLegacy struct {
	*Meta

	// Name identifies the user-defined metadata.
	Name string `xml:"name,attr"`
	// Content is the value of the metadata.
	Content string `xml:"content,attr"`
}

// Link element is used to associate resources with the given Rendition, such
// as metadata records.
type Link struct {
	// Href is an absolute or relative IRI reference [RFC3987] to a
	// resource.
	Href string `xml:"href,attr"`
	// ID attributes provides the ID [XML] of the element, which MUST be
	// unique within the document scope.
	ID string `xml:"id,attr,omitempty"`
	// MediaType indicates the MIME media type the Publication Resource
	// identified by Item MUST conform to.
	MediaType string `xml:"media-type,attr,omitempty"`
	// Properties takes a space-separated list of property values.
	Properties string `xml:"properties,attr,omitempty"`
	// Refines identifies the expression or resource augmented by the
	// element. The value of the attribute must be a relative IRI
	// [RFC3987] referencing the resource or element being described.
	Refines string `xml:"refines,attr,omitempty"`
	// Rel attribute takes a space-separated list of property values that
	// establish the relationship the resource has with the Rendition.
	Rel string `xml:"rel,attr"`
}

// Manifest element provides an exhaustive list of the Publication Resources
// that constitute the given Rendition, each represented by an item element.
type Manifest struct {
	// ID attributes provides the ID [XML] of the element, which MUST be
	// unique within the document scope.
	ID string `xml:"id,attr,omitempty"`
	// Items lists Publication Resources
	Items []Item `xml:"item"`
}

// Item element represents a Publication Resource.
type Item struct {
	// Fallback attribute takes an IDREF [XML] that identifies a
	// fallback for the Publication Resource referenced from the item
	// element.
	Fallback string `xml:"fallback,attr,omitempty"`
	// Href is an absolute or relative IRI reference [RFC3987] to a
	// resource.
	Href string `xml:"href,attr"`
	// ID attributes provides the ID [XML] of the element, which MUST be
	// unique within the document scope.
	ID string `xml:"id,attr"`
	// MediaOverlay attribute takes an IDREF [XML] that identifies
	// the Media Overlay Document for the resource described by this
	// item.
	MediaOverlay string `xml:"media-overlay,attr,omitempty"`
	// MediaType indicates the MIME media type the Publication Resource
	// identified by Item MUST conform to.
	MediaType string `xml:"media-type,attr"`
	// Properties is a space-separated list of property values.
	Properties string `xml:"properties,attr,omitempty"`
}

// Spine element defines an ordered list of manifest item references that
// represents the default reading order of the given Rendition.
type Spine struct {
	// ID attributes provides the ID [XML] of the element, which MUST be
	// unique within the document scope.
	ID string `xml:"id,attr,omitempty"`
	// PageProgression attribute sets the global direction in which the
	// content flows. Allowed values are ltr (left-to-right), rtl
	// (right-to-left) and default.
	PageProgression string `xml:"page-progression-direction,attr,omitempty"`
	// Toc is a legacy feature that previously provided the table of
	// contents for EPUB Publications.
	Toc string `xml:"toc,attr,omitempty"`
	// Itemrefs lists Publication Resources. The order of the Itemrefs
	// elements defines the default reading order of the given Rendition.
	Itemrefs []Itemref `xml:"itemref"`
}

// Itemref element represents a Publication Resource.
type Itemref struct {
	// ID attributes provides the ID [XML] of the element, which MUST be
	// unique within the document scope.
	ID string `xml:"id,attr,omitempty"`
	// IDref references the ID [XML] of a unique item in the manifest via
	// the IDREF [XML] in its idref attribute (i.e., two or more itemref
	// elements cannot reference the same item).
	IDref string `xml:"idref,attr"`
	// Linear attribute indicates whether the referenced item contains
	// content that contributes to the primary reading order and has to
	// be read sequentially ("yes") or auxiliary content that enhances or
	// augments the primary content and can be accessed out of sequence
	// ("no").
	Linear string `xml:"linear,attr,omitempty"`
	// Properties is a space-separated list of property values.
	Properties string `xml:"properties,attr,omitempty"`
}

// Collection element defines a related group of resources.
type Collection struct {
	// Dir attribute specifies the base text direction of the content and
	// attribute values of the carrying element and its descendants.
	// Allowed values are ltr (left-to-right) and rtl (right-to-left).
	Dir string `xml:"dir,attr,omitempty"`
	// ID attributes provides the ID [XML] of the element, which MUST be
	// unique within the document scope.
	ID string `xml:"id,attr,omitempty"`
	// Role uniquely identifies all conformant collection elements.
	Role string `xml:"role,attr"`
	// Lang specifies the language used in the contents and attribute
	// values of the carrying element and its descendants.
	Lang string `xml:"xml:lang,attr,omitempty"`

	Metadata    *Metadata    `xml:"metadata,omitempty"`
	Collections []Collection `xml:"collection,omitempty"`
	Links       []Link       `xml:"link,omitempty"`
}

// newPackageDocument creates an PackageDocument from an OPF content.
func newPackageDocument(r io.Reader) (*PackageDocument, error) {
	opf := &PackageDocument{}

	if err := decodeXML(r, &opf); err != nil {
		return nil, err
	}
	return opf, nil
}
