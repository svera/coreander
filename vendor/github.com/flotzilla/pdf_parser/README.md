Pdf metadata parser
====
Go library for parsing pdf metadata information 
 
### License
MIT 

### Usage
```go
import "github.com/flotzilla/pdf_parser"

// parse file
pdf, errors := pdf_parser.ParsePdf("filepath/file.pdf")

// main functions
pdf.GetTitle()
pdf.GetAuthor()
pdf.GetCreator()
pdf.GetISBN()
pdf.GetPublishers() []string
pdf.GetLanguages() []string
pdf.GetDescription()
pdf.GetPagesCount()
```

Using with custom `github.com/sirupsen/logrus` logger

```go
import "github.com/flotzilla/pdf_parser"

l := logger.New()
l.SetOutput(os.Stdout)
lg.SetFormatter(&logger.JSONFormatter{})

SetLogger(lg)
file, _ := filepath.Abs("filepath/file.pdf")
pdf, err := ParsePdf(file)

```
