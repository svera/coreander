# Coreander
A personal Ebooks server, Coreander indexes the ebooks (only in Epub format at the moment) that it finds in the passed folder, and provides a web interface to search and access them.

**Warning**
This code has been quickly hacked away during holidays and lacks proper testing. Although I've been using it extensively without problems, you may find issues, so use it at your own risk.

## Features
* Fast search engine powered by Bleve, with support for ebooks in multiple languages.
* High-performance web server powered by Fiber.
* Lightweight, responsive web interface based on Bootstrap.

## How to use
Only source code is provided at the moment, so you'll have to manually build it. The only requirement is Go 1.15.
It should run in any Go supported platform, but it has been only tested in Mac OS 10.15 (Catalina)

Clone the repo and, from its directory, then run `go build` and that's it. Note that if you want to move the generated binary to a different directory, both the `views` and `public` folders must be copied as well.

Coreander requires that a `config.yml` file to be present in your `$home/coreander` directory. A sample one is distributed with this source code. The main setting there is `library-path`, which tells the application where your books are located.