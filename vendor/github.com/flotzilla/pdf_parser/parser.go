package pdf_parser

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	logger "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var (
	log          = logger.New()
	isLogEnabled = false
)

// Set logrus logger to pdf parser instance
func SetLogger(logrusLogger *logger.Logger) {
	isLogEnabled = true
	log = logrusLogger
}

// Parse pdf file metadata
func ParsePdf(fileName string) (*PdfInfo, error) {
	pdfInfo := PdfInfo{}

	file, err := os.Open(fileName)
	if err != nil {
		logError(err)
		return &pdfInfo, err
	}

	defer file.Close()

	version, err := readPdfInfoVersion(file)
	if err != nil {
		logError(err)
		return &pdfInfo, err
	}

	pdfInfo.PdfVersion = version

	pdfInfo.PagesCount = countPages(file)

	err = readXrefOffset(file, &pdfInfo)
	if err != nil {
		logError(err)
		return &pdfInfo, err
	}

	getTrailerSection(file, &pdfInfo)

	// original xref
	err, parsedXref, trailerSection := readXrefBlock(file, pdfInfo.OriginalXrefOffset, true)
	if err != nil {
		logError(err)
		return &pdfInfo, err
	}
	pdfInfo.XrefTable = append(pdfInfo.XrefTable, parsedXref)
	pdfInfo.AdditionalTrailerSection = append(pdfInfo.AdditionalTrailerSection, trailerSection)

	readAllXrefSections(file, &pdfInfo, pdfInfo.OriginalTrailerSection.Prev)

	if trailerSection != nil {
		readAllXrefSections(file, &pdfInfo, trailerSection.Prev)
	}

	root := findRootObject(&pdfInfo, file)
	if root == nil {
		err = cannotFindRootObject
		logError(err)
		return &pdfInfo, nil
	}
	pdfInfo.Root = *root

	info := searchInfoSection(&pdfInfo, file)
	if info == nil {
		err = cannotFindInfoObject
		logError(err)
		return &pdfInfo, nil
	}
	pdfInfo.Info = *info

	meta, err := findMetadataObject(&pdfInfo, file)
	logError(err)
	if meta != nil {
		pdfInfo.Metadata = *meta
	}

	return &pdfInfo, nil
}

func parseBlockStream(b []byte) ([]byte, error) {
	s := bytes.Index(b, []byte("stream"))
	e := bytes.Index(b, []byte("endstream"))
	if s != -1 && e != -1 {
		s = s + len("stream")
		if b[s] == 13 || b[s] == 10 {
			s++
		}
		return b[s:e], nil
	}
	return nil, cannotFindStreamContent
}

func getFlatDecodeContent(b []byte) (string, error) {
	if bytes.Contains(b, []byte("/Filter [/FlateDecode]")) {
		parsedStream, err := parseBlockStream(b)
		str, err := decodeZippedData(parsedStream)

		if err != nil {
			return "", err
		}

		return str, nil
	}

	return "", unsupportedParseContent
}

// Get dcd content (img file mostly)
func getDcdDecodeContent(b []byte) []byte {
	if bytes.Contains(b, []byte("/Filter [/DCTDecode]")) &&
		bytes.Contains(b, []byte("/Subtype /Image")) {

		parsedStream, err := parseBlockStream(b)
		logError(err)
		return parsedStream
	}
	return nil
}

// absolute filepath should contains file name and extension, e.g. /home/get/some.jpg
func writeStreamToFile(b *[]byte, path string) int {
	f, err := os.Create(path)
	logError(err)
	defer f.Close()

	bytesWritten, err := f.Write(*b)

	logError(err)

	return bytesWritten
}

func decodeZippedData(data []byte) (string, error) {
	b := bytes.NewReader(data)

	r, err := zlib.NewReader(b)
	if err != nil {
		return "", err
	}
	resp, err := ioutil.ReadAll(r)

	err = r.Close()
	if err != nil {
		return "", err
	}

	return string(resp), nil
}

func findRootObject(pdfInfo *PdfInfo, file *os.File) *RootObject {
	for _, el := range pdfInfo.AdditionalTrailerSection {
		if el.Root.ObjectNumber != 0 {
			obj, err := readXrefObjectContent(el.Root.ObjectNumber, pdfInfo, file)
			if err != nil {
				logError(err)
			}

			parsedObj, err := parseObjectContent(obj)
			if err != nil {
				logError(err)
			}
			return parseRootObject(parsedObj)
		}
	}
	return nil
}

func parseRootObject(obj string) *RootObject {
	root := RootObject{}

	var (
		typeRegex      = regexp.MustCompile(`\/Type( )?(\/\w+)`)
		pagesRegex     = regexp.MustCompile(`\/Pages( )?(\d+)\s([\d]+)\s(\w)`)
		metadataRegex  = regexp.MustCompile(`\/Metadata( )?(\d+)\s([\d]+)\s(\w)`)
		pageLabelRegex = regexp.MustCompile(`\/PagesLabel( )?(\d+)\s([\d]+)\s(\w)`)
		langRegex      = regexp.MustCompile(`\/Lang( )?\((\w+-\w+)\)( )?`)
	)

	typeData := typeRegex.FindAllStringSubmatchIndex(obj, -1)
	pagesData := pagesRegex.FindAllStringSubmatchIndex(obj, -1)
	metaData := metadataRegex.FindAllStringSubmatchIndex(obj, -1)
	pLabelData := pageLabelRegex.FindAllStringSubmatchIndex(obj, -1)
	langData := langRegex.FindAllStringSubmatchIndex(obj, -1)

	if typeData != nil {
		root.Type = strings.TrimSpace(obj[typeData[0][4]:typeData[0][5]])
	}

	if len(langData) > 0 && len(langData[0]) == 8 {
		root.Lang = strings.TrimSpace(obj[langData[0][4]:langData[0][5]])
	}

	if len(pagesData) > 0 && len(pagesData[0]) == 10 {
		oi, err := parseObjectIdentifierFromString(obj[pagesData[0][3]:pagesData[0][9]])
		logError(err)
		root.Pages = oi
	}

	if len(metaData) > 0 && len(metaData[0]) == 10 {
		oi, err := parseObjectIdentifierFromString(obj[metaData[0][3]:metaData[0][9]])
		logError(err)
		root.Metadata = oi
	}

	if len(pLabelData) > 0 && len(pLabelData[0]) == 10 {
		oi, err := parseObjectIdentifierFromString(obj[pLabelData[0][3]:pLabelData[0][9]])
		logError(err)
		root.PageLabels = oi
	}

	return &root
}

func searchInfoSection(pdfInfo *PdfInfo, file *os.File) *InfoObject {
	for _, el := range pdfInfo.AdditionalTrailerSection {
		if el.Info.ObjectNumber != 0 {
			obj, err := readXrefObjectContent(el.Info.ObjectNumber, pdfInfo, file)
			if err != nil {
				logError(err)
			}

			parsedObj, err := parseObjectContent(obj)
			if err != nil {
				logError(err)
			}
			return parseInfoObject(parsedObj)
		}
	}
	return nil
}

func parseInfoObject(objectContent string) *InfoObject {
	var (
		creationDateReg = regexp.MustCompile(`(?m)(\/CreationDate( )?\()([^\)]*)`)
		producerReg     = regexp.MustCompile(`(?m)(\/Producer( )?\()([^\)]*)`)
		creatorReg      = regexp.MustCompile(`(?m)(\/Creator( )?\()([^\)]*)`)
		titleReg        = regexp.MustCompile(`(?m)(\/Title( )?\()([^\)]*)`)
		modDateReg      = regexp.MustCompile(`(?m)(\/ModDate( )?\()([^\)]*)`)
		authorReg       = regexp.MustCompile(`(?m)(\/Author( )?\()([^\)]*)`)
		info            = InfoObject{}
	)

	info.CreationDate = parseInfoObjRegex(&objectContent, creationDateReg)
	info.Producer = parseInfoObjRegex(&objectContent, producerReg)
	info.Creator = parseInfoObjRegex(&objectContent, creatorReg)
	info.Title = parseInfoObjRegex(&objectContent, titleReg)
	info.ModDate = parseInfoObjRegex(&objectContent, modDateReg)
	info.Author = parseInfoObjRegex(&objectContent, authorReg)

	return &info
}

func findMetadataObject(pdfInfo *PdfInfo, file *os.File) (*Metadata, error) {
	if pdfInfo.Root.Metadata != nil && pdfInfo.Root.Metadata.ObjectNumber != 0 {
		obj, err := readXrefObjectContent(pdfInfo.Root.Metadata.ObjectNumber, pdfInfo, file)
		logError(err)
		return parseMetadataContent(obj)
	}

	return nil, cannotFindObjectById
}

func parseMetadataContent(b []byte) (*Metadata, error) {
	meta := Metadata{}

	streamContent, err := parseBlockStream(b)
	if err != nil {
		return nil, err
	}
	meta.RawStreamData = streamContent
	meta.RdfMeta = parseRdfContent(&meta.RawStreamData)

	var (
		typeReg    = regexp.MustCompile(`(?m)\/Type( )?\/(\w+)`)
		subtypeReg = regexp.MustCompile(`(?m)\/Subtype( )?\/(\w+)`)
		lengthReg  = regexp.MustCompile(`(?m)\/Length( )?(\d+)`)
		DlReg      = regexp.MustCompile(`(?m)\/DL( )?(\d+)`)
	)

	typeRes := typeReg.FindAllSubmatchIndex(b, -1)
	subtypeRes := subtypeReg.FindAllSubmatchIndex(b, -1)
	lenRes := lengthReg.FindAllSubmatchIndex(b, -1)
	dlRes := DlReg.FindAllSubmatchIndex(b, -1)

	if len(typeRes) > 0 && len(typeRes[0]) == 6 {
		meta.Type = string(b[typeRes[0][4]:typeRes[0][5]])
	}

	if len(subtypeRes) > 0 && len(subtypeRes[0]) == 6 {
		meta.Subtype = string(b[subtypeRes[0][4]:subtypeRes[0][5]])
	}

	if len(lenRes) > 0 && len(lenRes[0]) == 6 {
		length, err := strconv.ParseInt(string(b[lenRes[0][4]:lenRes[0][5]]), 10, 64)
		if err != nil {
			return nil, err
		}
		meta.Length = length
	}

	if len(dlRes) > 0 && len(dlRes[0]) == 6 {
		dl, err := strconv.ParseInt(string(b[dlRes[0][4]:dlRes[0][5]]), 10, 64)
		if err != nil {
			return nil, err
		}
		meta.DL = dl
	}
	return &meta, nil
}

func parseRdfContent(b *[]byte) *MetaDataRdf {
	var (
		titleReg   = regexp.MustCompile(`(?m)\<dc:title>((.|\n)*)\<rdf:li(.*)\>((.|\n)*)\<\/rdf:li\>((.|\n)*)\<\/dc:title>`)
		descrReg   = regexp.MustCompile(`(?m)\<dc:description>((.|\n)*)\<rdf:li(.*)\>((.|\n)*)\<\/rdf:li\>((.|\n)*)\<\/dc:description>`)
		creatorReg = regexp.MustCompile(`(?m)\<dc:creator>((.|\n)*)\<rdf:li(.*)\>((.|\n)*)\<\/rdf:li\>((.|\n)*)\<\/dc:creator>`)
		dateReg    = regexp.MustCompile(`(?m)\<dc:date>((.|\n)*)\<rdf:li(.*)\>((.|\n)*)\<\/rdf:li\>((.|\n)*)\<\/dc:date>`)

		publisherBagElReg = regexp.MustCompile(`(?m)\<dc:publisher>((.|\n)*)\<rdf:Bag>((.|\n)*)\<\/rdf:Bag\>((.|\n)*)\<\/dc:publisher>`)
		languageBagReg    = regexp.MustCompile(`(?m)\<dc:language>((.|\n)*)\<rdf:Bag>((.|\n)*)\<\/rdf:Bag\>((.|\n)*)\<\/dc:language>`)

		isbnPrismReg = regexp.MustCompile(`(?m)\<prism:isbn\>(.*)\<\/prism:isbn\>`)
		isbnPdfxReg  = regexp.MustCompile(`(?m)\<pdfx:isbn\>(.*)\<\/pdfx:isbn\>`)
		meta         = MetaDataRdf{}
	)

	meta.Title = parseSingleRdfItem(titleReg, b)
	meta.Description = parseSingleRdfItem(descrReg, b)
	meta.Creator = parseSingleRdfItem(creatorReg, b)
	meta.Date = parseSingleRdfItem(dateReg, b)
	meta.Publishers = parseBagElements(publisherBagElReg, b)
	meta.Languages = parseBagElements(languageBagReg, b)

	if isbnPrism := isbnPrismReg.FindAllSubmatchIndex(*b, -1); len(isbnPrism) > 0 && len(isbnPrism[0]) == 4 {
		meta.Isbn = string((*b)[isbnPrism[0][2]:isbnPrism[0][3]])
	} else if isbnPrism := isbnPdfxReg.FindAllSubmatchIndex(*b, -1); len(isbnPrism) > 0 && len(isbnPrism[0]) == 4 {
		meta.Isbn = string((*b)[isbnPrism[0][2]:isbnPrism[0][3]])
	}

	return &meta
}

func parseSingleRdfItem(reg *regexp.Regexp, b *[]byte) string {
	titleRegResp := reg.FindAllSubmatchIndex(*b, -1)
	if len(titleRegResp) > 0 && len(titleRegResp[0]) == 16 {
		return string((*b)[titleRegResp[0][8]:titleRegResp[0][9]])
	}
	return ""
}

func parseBagElements(bagElRegex *regexp.Regexp, b *[]byte) []string {
	var elements []string
	if publisherBag := bagElRegex.FindAllSubmatchIndex(*b, -1); len(publisherBag) > 0 && len(publisherBag[0]) > 0 {
		publishers := string((*b)[publisherBag[0][6]:publisherBag[0][7]])
		bagElReg := regexp.MustCompile(`(?m)\<rdf:li>(.*)\<\/rdf:li>`)
		for _, match := range bagElReg.FindAllStringSubmatch(publishers, -1) {
			if len(match) == 2 {
				elements = append(elements, match[1])
			}
		}
	}

	return elements
}

func parseInfoObjRegex(objectContext *string, regex *regexp.Regexp) string {
	res := regex.FindAllStringSubmatch(*objectContext, -1)
	if len(res) > 0 && len(res[0]) == 4 {
		return res[0][3]
	}
	return ""
}

func readXrefObjectContent(objectNumber int, pdfInfo *PdfInfo, file *os.File) ([]byte, error) {
	var offset int64 = 0

	for _, xrefTable := range pdfInfo.XrefTable {
		if xrefTable == nil {
			// TODO fix for object xref
			return nil, invalidXrefTableStructure
		}
		if obj, ok := xrefTable.Objects[objectNumber]; ok {
			offset = int64(obj.ObjectNumber)
		}
	}

	if offset == 0 {
		return nil, cannotFindObjectById
	}

	var (
		bSize      int64 = 100
		data       []byte
		blocksRead = 0
	)

	for buffer := make([]byte, bSize); ; blocksRead++ {
		bytesRead, err := file.ReadAt(buffer, offset)
		if err != nil {
			if err == io.EOF {
				return nil, cannotParseObject
			}
			return nil, err
		}

		x := (buffer)[:bytesRead]
		if blocksRead == 0 && !bytes.Contains(x, []byte("obj")) {
			return nil, cannotParseObject
		}

		data = append(data, x...)
		offset += bSize

		if bytes.Contains(x, []byte("endobj")) {
			break
		}
	}

	return data, nil
}

func parseObjectContent(block []byte) (string, error) {
	s := bytes.Index(block, []byte("<<"))
	e := bytes.LastIndex(block, []byte(">>"))

	s = s + len("<<")
	if s == -1 || e == -1 {
		return "", invalidSearchIndex
	}
	if block[s] == 13 || block[s] == 10 {
		s++
	}

	return string(block[s:e]), nil
}

func readAllXrefSections(file *os.File, pdfInfo *PdfInfo, prevOffset int64) {
	if prevOffset != 0 {
		err, additionalXref, trailer := readXrefBlock(file, prevOffset, true)
		logError(err)
		pdfInfo.XrefTable = append(pdfInfo.XrefTable, additionalXref)
		pdfInfo.AdditionalTrailerSection = append(pdfInfo.AdditionalTrailerSection, trailer)

		if trailer != nil {
			readAllXrefSections(file, pdfInfo, trailer.Prev)
		}
	}
}

func readXrefOffset(file *os.File, pdfInfo *PdfInfo) error {
	buffer := make([]byte, BufferSize)
	var startXrefOffset = ""

	stat, err := file.Stat()
	if err != nil {
		return err
	}
	bytesRead, err := file.ReadAt(buffer, stat.Size()-BufferSize)
	if err != nil {
		return err
	}

	hexBytes, hexBytesWritten := binToHex(bytesRead, &buffer)

	r1 := "(737461727478726566)(0a|0d0a|0d)([a-fa-f0-9]+)(0d0a)(2525)(454f46)"
	r2 := "(737461727478726566)(0a|0d0a|0d)([a-fa-f0-9]+)(0d)(2525)(454f46)"
	r3 := "(737461727478726566)(0a|0d0a|0d)([a-fa-f0-9]+)(0a)(2525)(454f46)"

	if r1Resp := parseRegex(r1, (*hexBytes)[:hexBytesWritten]); r1Resp != nil {
		startXrefOffset, err = decodeHexAsString((*hexBytes)[:hexBytesWritten][r1Resp[0][6]:r1Resp[0][8]])
	} else if r2Resp := parseRegex(r2, (*hexBytes)[:hexBytesWritten]); r2Resp != nil {
		startXrefOffset, err = decodeHexAsString((*hexBytes)[:hexBytesWritten][r2Resp[0][5]:r2Resp[0][7]])
	} else if r3Resp := parseRegex(r3, (*hexBytes)[:hexBytesWritten]); r3Resp != nil {
		startXrefOffset, err = decodeHexAsString((*hexBytes)[:hexBytesWritten][r3Resp[0][5]:r3Resp[0][7]])
	} else {
		return cannotReadXrefOffset
	}

	if err != nil {
		return err
	}

	intVal, err := strconv.ParseInt(startXrefOffset, 10, 64)
	if err != nil {
		return cannotReadXrefOffset
	}

	pdfInfo.OriginalXrefOffset = intVal
	return nil
}

func readXrefBlock(file *os.File, xrefOffset int64, trailerRead bool) (error, *XrefTable, *TrailerSection) {
	if xrefOffset == -1 || xrefOffset == 0 {
		return cannotParseXrefOffset, nil, nil
	}

	var bSize int64 = 100
	buffer := make([]byte, bSize)

	offset := xrefOffset
	var xrefBlock [][]byte
	var additionalTrailer TrailerSection

	for {
		bytesRead, err := file.ReadAt(buffer, offset)
		if err != nil {
			if err == io.EOF {
				hexBytes, hexBytesWritten := binToHex(bytesRead, &buffer)
				xrefBlock = append(xrefBlock, (*hexBytes)[:hexBytesWritten])

				break
			}
			return err, nil, nil
		}

		hexBytes, hexBytesWritten := binToHex(bytesRead, &buffer)
		xrefBlock = append(xrefBlock, (*hexBytes)[:hexBytesWritten])

		if trailerBlockFound(*hexBytes) || checkXrefTrailer(xrefBlock) {
			if trailerRead {
				additionalTrailer = *getFileTrailer(file, offset-bSize)
			}
			break
		}
		offset += bSize
	}

	// read Xref
	var remaining []byte
	var xrefSectionsString []string
	var trailerCounter = 0
	var trailerStartBlock []byte
	for c, el := range xrefBlock {
		if trailerBlockFound(el) {
			var preTrailer []byte
			preTrailer, trailerStartBlock = getPreTrailerData(el)
			err := readXrefBlockSection(&remaining, &preTrailer, &xrefSectionsString)
			if err != nil {
				return err, nil, nil
			}
			trailerCounter = c
			break
		} else {
			err := readXrefBlockSection(&remaining, &el, &xrefSectionsString)
			if err != nil {
				return err, nil, nil
			}
		}
	}

	var xrefBig []byte
	for i := trailerCounter; i < len(xrefBlock); i++ {
		if i == trailerCounter {
			xrefBig = append(xrefBig, trailerStartBlock...)
			continue
		}
		xrefBig = append(xrefBig, xrefBlock[i]...)
	}

	parsedXref, err := parseXrefSection(&xrefSectionsString)
	if err != nil {
		return err, nil, nil
	}

	parsedXref.SectionStart = xrefOffset
	return nil, parsedXref, &additionalTrailer
}

func parseXrefSection(elements *[]string) (*XrefTable, error) {
	regexpSubsection := regexp.MustCompile(`(?m)(\d+)\s([\d]+)( )?$`)
	regexSubsectionElement := regexp.MustCompile(`(?m)(\d+)\s([\d]+)\s(\w)`)

	isSectionFound := false
	objectSubsectionCount := 0
	xref := XrefTable{
		make(map[int]*ObjectSubsectionElement),
		make(map[int]*ObjectSubsection),
		0,
	}
	var currentObjectSection *ObjectSubsection

	for _, el := range *elements {
		if el == "xref" {
			continue
		}
		if el == "trailer" {
			break
		}

		if regexpSubsection.MatchString(el) {
			foundItem := regexpSubsection.FindAllStringSubmatch(el, -1)

			id, err := strconv.Atoi(foundItem[0][1])
			if err != nil {
				return nil, err
			}
			count, err := strconv.Atoi(foundItem[0][2])
			if err != nil {
				return nil, err
			}

			isSectionFound = true
			objectSubsectionCount = 0

			subsection := ObjectSubsection{
				Id:                      id,
				ObjectsCount:            count,
				FirstSubsectionObjectId: id,
				LastSubsectionObjectId:  id + count - 1,
				Elements:                make(map[int]*ObjectSubsectionElement),
			}

			currentObjectSection = &subsection
			xref.ObjectSubsections[id] = &subsection

		} else if regexSubsectionElement.MatchString(el) {
			if isSectionFound && currentObjectSection != nil {
				foundItem := regexSubsectionElement.FindAllStringSubmatch(el, -1)
				objNum, err := strconv.Atoi(foundItem[0][1])
				if err != nil {
					return nil, err
				}
				genNum, err := strconv.Atoi(foundItem[0][2])
				if err != nil {
					return nil, err
				}

				currId := currentObjectSection.Id + objectSubsectionCount

				oe := ObjectSubsectionElement{
					currId,
					objNum,
					genNum,
					foundItem[0][3],
				}
				currentObjectSection.Elements[currId] = &oe
				xref.Objects[currId] = &oe
				objectSubsectionCount++

			} else {
				return nil, cannotParseXrefSection
			}
		} else {
			return nil, cannotParseXrefSection
		}
	}
	return &xref, nil
}

func readXrefBlockSection(remaining *[]byte, block *[]byte, xrefSections *[]string) error {
	if len(*remaining) > 0 {
		err := readTillEndLine(append(*remaining, *block...), remaining, xrefSections)
		if err != nil {
			return err
		}
	} else {
		err := readTillEndLine(*block, remaining, xrefSections)
		if err != nil {
			return err
		}
	}
	return nil
}

func readTillEndLine(block []byte, remainingBlock *[]byte, xrefSections *[]string) error {
	r1 := "(?mU)([a-fA-F0-9])(0d0a|0d|0a)"
	regex := parseRegex(r1, block)
	for match := range regex {
		if match == 0 {
			str, err := decodeHexAsString(block[:regex[match][4]])
			if err != nil {
				return err
			}
			*xrefSections = append(*xrefSections, str)
		} else if match > 0 && match < len(regex)-1 {
			str, err := decodeHexAsString(block[regex[match-1][5]:regex[match][4]])
			if err != nil {
				return err
			}
			*xrefSections = append(*xrefSections, str)
		} else {
			str, err := decodeHexAsString(block[regex[match-1][5]:regex[match][4]])
			if err != nil {
				return err
			}
			*xrefSections = append(*xrefSections, str)

			str, err = decodeHexAsString(block[regex[match][5]:])
			if err != nil {
				return err
			}
			*remainingBlock = block[regex[match][5]:]
		}
	}

	return nil
}

func trailerBlockFound(block []byte) bool {
	xrefRegex := "747261696c6572"
	matched, _ := regexp.Match(xrefRegex, block)

	return matched
}

func checkXrefTrailer(xref [][]byte) bool {
	var newBlock []byte
	xrefLen := len(xref)
	if xrefLen > 1 {
		newBlock = append(xref[xrefLen-2][6:], xref[xrefLen-1][:6]...)
		return trailerBlockFound(newBlock)
	}

	return false
}

func getPreTrailerData(block []byte) ([]byte, []byte) {
	reg := "(?mU)([a-fA-F0-9]*)747261696c6572(0d0a|0d|0a)([a-fA-F0-9]*)$"
	resp := parseRegex(reg, block)

	return block[:resp[0][3]], block[resp[0][3]:]
}

func decodeHex(hexVal []byte) ([]byte, error) {
	intv := make([]byte, hex.DecodedLen(len(hexVal)))
	_, err := hex.Decode(intv, []byte(hexVal))

	if err != nil {
		return make([]byte, 0), err
	}

	return intv, nil
}

func decodeHexAsString(hexVal []byte) (string, error) {
	decoded, err := decodeHex(hexVal)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func parseRegex(regexStr string, bytes []byte) [][]int {
	r := regexp.MustCompile(regexStr)
	return r.FindAllSubmatchIndex(bytes, -1)
}

func logError(args ...interface{}) {
	if isLogEnabled {
		for _, n := range args {
			log.Error(n)
		}
	}
}

func logWarn(args ...interface{}) {
	if isLogEnabled {
		for _, n := range args {
			log.Warn(n)
		}
	}
}

func binToHex(bytesRead int, buffer *[]byte) (*[]byte, int) {
	dst := make([]byte, hex.EncodedLen(bytesRead))
	n := hex.Encode(dst, (*buffer)[:bytesRead])

	return &dst, n
}

func readPdfInfoVersion(file *os.File) (string, error) {
	buffer := make([]byte, 15)

	_, err := file.Seek(0, 0)
	if err != nil {
		return "", err
	}
	bytesread, err := file.Read(buffer)

	dst, n := binToHex(bytesread, &buffer)
	pdfVersion, err := getPdfVersion((*dst)[:n])

	if err != nil {
		return "", err
	}

	return pdfVersion, nil
}

func getPdfVersion(content []byte) (string, error) {
	r1 := regexp.MustCompile("(09|25)(5044462d)")
	var pdfVersion = ""
	if r1.Match(content) {
		re := regexp.MustCompile("(09|25)(5044462d)([a-f0-9]+)(0a|0d)25")
		indx := re.FindAllSubmatchIndex(content, -1)

		if indx != nil {
			intv, err := decodeHex(content[indx[0][5]:indx[0][8]])

			if err != nil {
				return "", err
			}
			pdfVersion = string(intv)

		}
		return pdfVersion, nil
	} else {
		return "", fileIsNotPdfError
	}
}

func getTrailerSection(file *os.File, pdfInfo *PdfInfo) {
	stat, err := file.Stat()
	logError(err)
	trailer := getFileTrailer(file, stat.Size()-BufferSize300)
	pdfInfo.OriginalTrailerSection = *trailer
}

func getFileTrailer(file *os.File, sectionStart int64) *TrailerSection {
	buffer := make([]byte, BufferSize300*2)
	bytesRead, err := file.ReadAt(buffer, sectionStart)
	if err != nil {
		if err == io.EOF {
			// continue
		}
	} else {
		logError(err)
	}
	hexBytes, hexBytesWritten := binToHex(bytesRead, &buffer)
	slice := (*hexBytes)[:hexBytesWritten]
	trailer, err := parseTrailerBlock(&slice)
	logError(err)
	return &trailer
}

func parseTrailerBlock(block *[]byte) (TrailerSection, error) {
	r1 := "(?mU)747261696c6572(0d0a|0d|0a)3c3c((0a|0d)?)([a-fA-F0-9]+)3e3e"
	if r1Resp := parseRegex(r1, *block); r1Resp != nil {
		trailerBlock := (*block)[r1Resp[0][8]:r1Resp[0][1]]
		var (
			trailer   = TrailerSection{}
			sizeRegex = "(?mU)2f53697a65(20)?([a-zA-Z0-9]+)(2f|3e3e)"
			idxRegex  = "(?mU)2f4944(20)?([a-zA-Z0-9]+)(2f|3e3e)"
			rootRegex = "(?mU)2f526f6f74(20)?([a-zA-Z0-9]+)(2f|3e3e)"
			infoRegex = "(?mU)2f496e666f(20)?([a-zA-Z0-9]+)(2f|3e3e)"
			prevRegex = "(?mU)2f50726576(20)?([a-zA-Z0-9]+)(2f|3e3e)"
		)

		size, err := parseTrailerSection(sizeRegex, &trailerBlock)
		logError(err)
		trailer.Size = strings.TrimSpace(size)

		index, err := parseTrailerSection(idxRegex, &trailerBlock)
		logError(err)
		trailer.IdRaw = index

		root, err := parseTrailerSection(rootRegex, &trailerBlock)
		logError(err)
		if root != "" {
			oi, err := parseObjectIdentifierFromString(root)
			logError(err)
			trailer.Root = *oi
		}

		info, err := parseTrailerSection(infoRegex, &trailerBlock)
		logError(err)
		if info != "" {
			inf, err := parseObjectIdentifierFromString(info)
			logError(err)
			trailer.Info = *inf
		}

		prev, err := parseTrailerSection(prevRegex, &trailerBlock)
		logError(err)
		if prev != "" {
			prev = strings.TrimSpace(prev)
			p, err := strconv.ParseInt(prev, 10, 64)
			logError(err)
			trailer.Prev = p
		}
		return trailer, nil
	}

	return TrailerSection{}, cannotParseTrailer
}

func parseTrailerSection(regex string, block *[]byte) (string, error) {
	if resp := parseRegex(regex, *block); resp != nil {
		return decodeHexAsString((*block)[resp[0][4]:resp[0][5]])
	}

	return "", nil
}

func parseObjectIdentifierFromString(str string) (*ObjectIdentifier, error) {

	r := regexp.MustCompile("(\\d+) (\\d+) ([\\S]+?)")
	res := r.FindAllStringSubmatch(str, -1)
	oi := ObjectIdentifier{}

	if res != nil {
		if oNumber, err := strconv.Atoi(res[0][1]); err == nil {
			oi.ObjectNumber = oNumber
		} else {
			return nil, err
		}

		if gNumber, err := strconv.Atoi(res[0][2]); err == nil {
			oi.GenerationNumber = gNumber
		} else {
			return nil, err
		}

		oi.KeyWord = res[0][3]
		return &oi, nil
	}

	return &oi, nil
}

func countPages(file *os.File) int {
	buffer := make([]byte, BufferSize300)
	reg := regexp.MustCompile(`(?m)\/Type( )?\/Page([^s])`)
	var (
		offset int64 = 0
		count  int   = 0
	)

	for {
		bytesRead, err := file.ReadAt(buffer, offset)
		chunk := (buffer)[:bytesRead]

		if err != nil {
			if err == io.EOF {
				resp := reg.FindAllSubmatch(chunk, -1)
				if resp != nil {
					count += len(resp)
				}
				break
			}
		}

		resp := reg.FindAllSubmatch(chunk, -1)
		if resp != nil {
			count += len(resp)
		}
		offset += BufferSize300
	}
	return count
}
