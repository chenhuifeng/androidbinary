package androidbinary

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// XMLFile is an XML file expressed in binary format.
type XMLFile struct {
	stringPool     *ResStringPool
	notPrecessedNS map[ResStringPoolRef]ResStringPoolRef
	namespaces     xmlNamespaces
	syntheticNS    map[ResStringPoolRef]string // URI ref -> prefix when START_NAMESPACE is missing
	pendingXMLNS   []xmlnsPending
	xmlBuffer      bytes.Buffer
}

type InvalidReferenceError struct {
	Ref ResStringPoolRef
}

func (e *InvalidReferenceError) Error() string {
	return fmt.Sprintf("androidbinary: invalid reference: 0x%08X", e.Ref)
}

type (
	xmlNamespaces struct {
		l []namespaceVal
	}
	namespaceVal struct {
		key   ResStringPoolRef
		value ResStringPoolRef
	}
	xmlnsPending struct {
		prefix string
		uri    string
	}
)

func (x *xmlNamespaces) add(key ResStringPoolRef, value ResStringPoolRef) {
	x.l = append(x.l, namespaceVal{key: key, value: value})
}

func (x *xmlNamespaces) remove(key ResStringPoolRef) {
	for i := len(x.l) - 1; i >= 0; i-- {
		if x.l[i].key == key {
			var newList = append(x.l[:i], x.l[i+1:]...)
			x.l = newList
			return
		}
	}
}

// get returns the prefix string-pool ref for a namespace URI.
// ok is false when the URI has not been introduced by a START_NAMESPACE chunk.
func (x *xmlNamespaces) get(key ResStringPoolRef) (ResStringPoolRef, bool) {
	for i := len(x.l) - 1; i >= 0; i-- {
		if x.l[i].key == key {
			return x.l[i].value, true
		}
	}
	return NilResStringPoolRef, false
}

// ResXMLTreeNode is basic XML tree node.
type ResXMLTreeNode struct {
	Header     ResChunkHeader
	LineNumber uint32
	Comment    ResStringPoolRef
}

// ResXMLTreeNamespaceExt is extended XML tree node for namespace start/end nodes.
type ResXMLTreeNamespaceExt struct {
	Prefix ResStringPoolRef
	URI    ResStringPoolRef
}

// ResXMLTreeAttrExt is extended XML tree node for start tags -- includes attribute.
type ResXMLTreeAttrExt struct {
	NS             ResStringPoolRef
	Name           ResStringPoolRef
	AttributeStart uint16
	AttributeSize  uint16
	AttributeCount uint16
	IDIndex        uint16
	ClassIndex     uint16
	StyleIndex     uint16
}

// ResXMLTreeAttribute is an attribute of start tags.
type ResXMLTreeAttribute struct {
	NS         ResStringPoolRef
	Name       ResStringPoolRef
	RawValue   ResStringPoolRef
	TypedValue ResValue
}

// ResXMLTreeEndElementExt is extended XML tree node for element start/end nodes.
type ResXMLTreeEndElementExt struct {
	NS   ResStringPoolRef
	Name ResStringPoolRef
}

// NewXMLFile returns a new XMLFile.
func NewXMLFile(r io.ReaderAt) (*XMLFile, error) {
	f := new(XMLFile)
	sr := io.NewSectionReader(r, 0, 1<<63-1)

	fmt.Fprintf(&f.xmlBuffer, xml.Header)

	header := new(ResChunkHeader)
	if err := binary.Read(sr, binary.LittleEndian, header); err != nil {
		return nil, err
	}
	offset := int64(header.HeaderSize)
	for offset < int64(header.Size) {
		chunkHeader, err := f.readChunk(r, offset)
		if err != nil {
			return nil, err
		}
		offset += int64(chunkHeader.Size)
	}
	return f, nil
}

// Reader returns a reader of XML file expressed in text format.
func (f *XMLFile) Reader() *bytes.Reader {
	return bytes.NewReader(f.xmlBuffer.Bytes())
}

// Decode decodes XML file and stores the result in the value pointed to by v.
// To resolve the resource references, Decode also stores default TableFile and ResTableConfig in the value pointed to by v.
func (f *XMLFile) Decode(v interface{}, table *TableFile, config *ResTableConfig) error {
	decoder := xml.NewDecoder(f.Reader())
	if err := decoder.Decode(v); err != nil {
		return err
	}
	inject(reflect.ValueOf(v), table, config)
	return nil
}

func (f *XMLFile) readChunk(r io.ReaderAt, offset int64) (*ResChunkHeader, error) {
	sr := io.NewSectionReader(r, offset, 1<<63-1-offset)
	chunkHeader := &ResChunkHeader{}
	if _, err := sr.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if err := binary.Read(sr, binary.LittleEndian, chunkHeader); err != nil {
		return nil, err
	}
	if chunkHeader.HeaderSize < uint16(binary.Size(chunkHeader)) {
		return nil, fmt.Errorf("androidbinary: invalid chunk header size: %d", chunkHeader.HeaderSize)
	}
	if chunkHeader.Size < uint32(chunkHeader.HeaderSize) {
		return nil, fmt.Errorf("androidbinary: invalid chunk size: %d", chunkHeader.Size)
	}

	var err error
	if _, err := sr.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	switch chunkHeader.Type {
	case ResStringPoolChunkType:
		f.stringPool, err = readStringPool(sr)
	case ResXMLResourceMapType:
		err = f.readResourceMap(sr)

	case ResXMLStartNamespaceType:
		err = f.readStartNamespace(sr)

	case ResXMLEndNamespaceType:
		err = f.readEndNamespace(sr)

	case ResXMLStartElementType:
		err = f.readStartElement(sr)

	case ResXMLEndElementType:
		err = f.readEndElement(sr)

	case ResXMLCDataType:
		err = f.readXmlCData(sr)

	case ResNullChunkType:
		err = f.readNull(sr)
	}

	if err != nil {
		return nil, err
	}

	return chunkHeader, nil
}

// GetString returns a string referenced by ref.
// It panics if the pool doesn't contain ref.
func (f *XMLFile) GetString(ref ResStringPoolRef) string {
	return f.stringPool.GetString(ref)
}

func (f *XMLFile) HasString(ref ResStringPoolRef) bool {
	return f.stringPool.HasString(ref)
}

func (f *XMLFile) readStartNamespace(sr *io.SectionReader) error {
	header := new(ResXMLTreeNode)
	if err := binary.Read(sr, binary.LittleEndian, header); err != nil {
		return err
	}

	if _, err := sr.Seek(int64(header.Header.HeaderSize), io.SeekStart); err != nil {
		return err
	}
	namespace := new(ResXMLTreeNamespaceExt)
	if err := binary.Read(sr, binary.LittleEndian, namespace); err != nil {
		return err
	}

	if f.notPrecessedNS == nil {
		f.notPrecessedNS = make(map[ResStringPoolRef]ResStringPoolRef)
	}
	f.notPrecessedNS[namespace.URI] = namespace.Prefix
	f.namespaces.add(namespace.URI, namespace.Prefix)
	return nil
}

func (f *XMLFile) readEndNamespace(sr *io.SectionReader) error {
	header := new(ResXMLTreeNode)
	if err := binary.Read(sr, binary.LittleEndian, header); err != nil {
		return err
	}

	if _, err := sr.Seek(int64(header.Header.HeaderSize), io.SeekStart); err != nil {
		return err
	}
	namespace := new(ResXMLTreeNamespaceExt)
	if err := binary.Read(sr, binary.LittleEndian, namespace); err != nil {
		return err
	}
	f.namespaces.remove(namespace.URI)
	return nil
}

func (f *XMLFile) addNamespacePrefix(ns, name ResStringPoolRef) (string, error) {
	if !f.HasString(name) {
		return "", &InvalidReferenceError{Ref: name}
	}
	local := f.GetString(name)

	if ns == NilResStringPoolRef {
		return local, nil
	}

	prefix, err := f.resolveNamespacePrefix(ns)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%s", prefix, local), nil
}

// resolveNamespacePrefix returns the XML prefix for a namespace URI ref.
// Modern aapt2 may omit RES_XML_START_NAMESPACE chunks; in that case the prefix
// is derived from the URI string (e.g. android schema → "android").
func (f *XMLFile) resolveNamespacePrefix(ns ResStringPoolRef) (string, error) {
	if ref, ok := f.namespaces.get(ns); ok {
		if !f.HasString(ref) {
			return "", &InvalidReferenceError{Ref: ref}
		}
		return f.GetString(ref), nil
	}
	if f.syntheticNS != nil {
		if prefix, ok := f.syntheticNS[ns]; ok {
			return prefix, nil
		}
	}
	if !f.HasString(ns) {
		return "", &InvalidReferenceError{Ref: ns}
	}
	uri := f.GetString(ns)
	prefix := prefixFromNamespaceURI(uri)
	if f.syntheticNS == nil {
		f.syntheticNS = make(map[ResStringPoolRef]string)
	}
	f.syntheticNS[ns] = prefix
	f.pendingXMLNS = append(f.pendingXMLNS, xmlnsPending{prefix: prefix, uri: uri})
	return prefix, nil
}

func prefixFromNamespaceURI(uri string) string {
	switch uri {
	case "http://schemas.android.com/apk/res/android":
		return "android"
	case "http://schemas.android.com/apk/res-auto":
		return "app"
	case "http://schemas.android.com/tools":
		return "tools"
	case "http://schemas.android.com/apk/distribution":
		return "dist"
	case "urn:oasis:names:tc:xliff:document:1.2":
		return "xliff"
	}
	// Fall back to the last non-empty path/segment of the URI.
	trimmed := strings.TrimRight(uri, "/")
	if i := strings.LastIndexAny(trimmed, "/:#"); i >= 0 && i+1 < len(trimmed) {
		seg := trimmed[i+1:]
		if seg != "" {
			return seg
		}
	}
	if uri != "" {
		return "ns"
	}
	return "ns"
}

func (f *XMLFile) flushPendingXMLNS() {
	for _, decl := range f.pendingXMLNS {
		fmt.Fprintf(&f.xmlBuffer, " xmlns:%s=\"", decl.prefix)
		xml.Escape(&f.xmlBuffer, []byte(decl.uri))
		fmt.Fprint(&f.xmlBuffer, "\"")
	}
	f.pendingXMLNS = nil
}

func (f *XMLFile) readStartElement(sr *io.SectionReader) error {
	header := new(ResXMLTreeNode)
	if err := binary.Read(sr, binary.LittleEndian, header); err != nil {
		return err
	}

	if _, err := sr.Seek(int64(header.Header.HeaderSize), io.SeekStart); err != nil {
		return err
	}
	ext := new(ResXMLTreeAttrExt)
	if err := binary.Read(sr, binary.LittleEndian, ext); err != nil {
		return nil
	}

	tag, err := f.addNamespacePrefix(ext.NS, ext.Name)
	if err != nil {
		return err
	}
	f.xmlBuffer.WriteString("<")
	f.xmlBuffer.WriteString(tag)

	// output XML namespaces
	if f.notPrecessedNS != nil {
		for uri, prefix := range f.notPrecessedNS {
			fmt.Fprintf(&f.xmlBuffer, " xmlns:%s=\"", f.GetString(prefix))
			xml.Escape(&f.xmlBuffer, []byte(f.GetString(uri)))
			fmt.Fprint(&f.xmlBuffer, "\"")
		}
		f.notPrecessedNS = nil
	}
	f.flushPendingXMLNS()

	// process attributes
	offset := int64(ext.AttributeStart + header.Header.HeaderSize)
	for i := 0; i < int(ext.AttributeCount); i++ {
		if _, err := sr.Seek(offset, io.SeekStart); err != nil {
			return err
		}
		attr := new(ResXMLTreeAttribute)
		binary.Read(sr, binary.LittleEndian, attr)

		var value string
		if attr.RawValue != NilResStringPoolRef {
			if !f.HasString(attr.RawValue) {
				return &InvalidReferenceError{Ref: attr.RawValue}
			}
			value = f.GetString(attr.RawValue)
		} else {
			data := attr.TypedValue.Data
			switch attr.TypedValue.DataType {
			case TypeNull:
				value = ""
			case TypeReference:
				value = fmt.Sprintf("@0x%08X", data)
			case TypeIntDec:
				value = fmt.Sprintf("%d", data)
			case TypeIntHex:
				value = fmt.Sprintf("0x%08X", data)
			case TypeIntBoolean:
				if data != 0 {
					value = "true"
				} else {
					value = "false"
				}
			default:
				value = fmt.Sprintf("@0x%08X", data)
			}
		}

		name, err := f.addNamespacePrefix(attr.NS, attr.Name)
		if err != nil {
			return err
		}
		f.flushPendingXMLNS()
		fmt.Fprintf(&f.xmlBuffer, " %s=\"", name)
		xml.Escape(&f.xmlBuffer, []byte(value))
		fmt.Fprint(&f.xmlBuffer, "\"")
		offset += int64(ext.AttributeSize)
	}
	fmt.Fprint(&f.xmlBuffer, ">")
	return nil
}

func (f *XMLFile) readEndElement(sr *io.SectionReader) error {
	header := new(ResXMLTreeNode)
	if err := binary.Read(sr, binary.LittleEndian, header); err != nil {
		return err
	}
	if _, err := sr.Seek(int64(header.Header.HeaderSize), io.SeekStart); err != nil {
		return err
	}
	ext := new(ResXMLTreeEndElementExt)
	if err := binary.Read(sr, binary.LittleEndian, ext); err != nil {
		return err
	}
	tag, err := f.addNamespacePrefix(ext.NS, ext.Name)
	if err != nil {
		return err
	}
	fmt.Fprintf(&f.xmlBuffer, "</%s>", tag)
	return nil
}

func (f *XMLFile) readResourceMap(sr *io.SectionReader) error {
	return nil
}

func (f *XMLFile) readXmlCData(sr *io.SectionReader) error {
	return nil
}

func (f *XMLFile) readNull(sr *io.SectionReader) error {
	return nil
}
