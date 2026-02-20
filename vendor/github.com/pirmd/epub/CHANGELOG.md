# Changelog
## [0.3.1] - 2024-12-19
- solve dependencies security issues in golang.org/x/text (CVE-2024-45338)
- remove problematic close in epub.go 

## [0.3.0] - 2023-02-26
- solve dependencies security issues in golang.org/x/text (CVE-2021-38561,
  CVE-2022-32149) and golang.org/x/net (CVE-2022-27664, CVE-2022-41721)
- remove use of ReadAtSeeker as a mean to access an EPUB's metadata
- create an Epub type to gather standard EPUB manipulation

## [0.2.0] - 2022-06-23
- expose PackageDocument struct and add functions to get it from an epub.
- add compliance to EPUB32 specifications https://www.w3.org/publishing/epub32/epub-packages.html.
- add helpers (WalkXxX) to access EPUB publication resources.

## [0.1.0] - 2019-05-11
### Added
- EPUB2 metadata reading.
- epub tool that print metadata from an epub file to the standard output.
