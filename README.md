# Coreander
A personal Ebooks server, Coreander indexes the ebooks (only in Epub format at the moment) that it finds in the passed folder, and provides a web interface to search and access them.

![Coreander screenshot](screenshot.png)

### Warning
**This code has been quickly hacked away during holidays and lacks proper testing. Although I've been using it extensively without problems, you may find issues, so use it at your own risk.**

## Features
* Fast search engine powered by [Bleve](https://github.com/blevesearch/bleve), with support for ebooks in multiple languages.
* High-performance web server powered by [Fiber](https://github.com/gofiber/fiber).
* Lightweight, responsive web interface based on [Bootstrap](https://getbootstrap.com/).

## Installation
Only source code is provided at the moment, so you'll have to manually build it. The only requirement is Go 1.15.
The application should build and run in any Go supported platform, but it has been only tested in Mac OS 10.15 (Catalina).

Clone the repo and, from its directory, run `go build` to generate the binary and then execute it with `coreander`. That's it. Note that if you want to move the generated binary to a different directory, both the `views` and `public` folders must be copied as well.

## How to use
Coreander requires a `LIBPATH` environment variable to be set, which tells the application where your books are located.

On first run, Coreander will index the books in your library, creating a database with those entries located at `$home/coreander/db`. Depending on your system's performance and the size of your library this may take a while. Also, the database can grow fairly big, so make sure you have enough free space on disk.

Even if the application is still indexing entries, you can access its web interface right away. Just open a web browser and go to `localhost:3000` (replace `localhost` for the IP address of the machine where the server is running if you want to access it from another machine). It is possible to change the listening port just executing the application with the `PORT` environment variable (e. g. `PORT=4000 coreander`) 