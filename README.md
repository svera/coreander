# Coreander
A personal Ebooks server, Coreander indexes the ebooks (only EPUBs with no DRM at the moment) that it finds in the passed folder, and provides a web interface to search and access them.

![Coreander screenshot](screenshot.png)

## Features
* Single binary with all dependencies included.
* Fast search engine powered by [Bleve](https://github.com/blevesearch/bleve), with support for ebooks in multiple languages.
* Search by author, title and even book series ([Calibre's](https://calibre-ebook.com/) `series` meta supported)
* Estimated reading time calculation. 
* High-performance web server powered by [Fiber](https://github.com/gofiber/fiber).
* Lightweight, responsive web interface based on [Bootstrap](https://getbootstrap.com/).
* Web interface available in English and Spanish, more languages can be easily added.
* New books added or removed to/from the library folder are automatically indexed (Linux only).
* [Send to email supported](#send-to-email).
* Read indexed epubs from Coreander's interface thanks to [epub.js](http://futurepress.org/).
* Restrictable access only to registered users.

## Building from source
Coreander's only requirement is Go 1.18.

There are two possibilites for building Coreander from source:
* If you have [Mage](https://magefile.org) installed in your system, just type `mage install` from the source code folder.
* Otherwise, a simple `go build` or `go install` will do, although no version information will be added to the executable.

## How to use
Coreander is designed to be run as a service managed by systemd or any other service manager. For example, in Raspbian, just create a file called `/etc/systemd/system/coreander.service` with the following contents:

```
[Unit]
Description=coreander

[Service]
Type=simple
Restart=always
RestartSec=5s
WorkingDirectory=<absolute path to directory which contains coreander binary>
ExecStart=<absolute path to coreander binary>
PermissionsStartOnly=true
StandardOutput=syslog
StandardError=syslog
SyslogIdentifier=sleepservice
User=<user which will execute this service>
Environment="LIBPATH=<absolute path to the library>"

```

then, start the service with `service coreander start`. You can manage it with the usual commands `start`, `stop` and `status`. Refer to your service manager documentation for more information.

Coreander requires a `LIBPATH` environment variable to be set, which tells the application where your books are located.

On first run, Coreander will index the books in your library, creating a database with those entries located at `$home/coreander/db`. Depending on your system's performance and the size of your library this may take a while. Also, the database can grow fairly big, so make sure you have enough free space on disk.

Every time is run, the application check for new entries, reindexing the whole library. You can
avoid this behaviour by setting the environment variable `SKIPREINDEX` to `true`. 

Even if the application is still indexing entries, you can access its web interface right away. Just open a web browser and go to `localhost:3000` (replace `localhost` for the IP address of the machine where the server is running if you want to access it from another machine). It is possible to change the listening port just executing the application with the `PORT` environment variable (e. g. `PORT=4000 coreander`)

### Send to email

Coreander can also send documents through email. This way, you can take advantage of services such as [Amazon's send to email](https://www.amazon.com/gp/help/customer/display.html?nodeId=G7NECT4B4ZWHQ8WV), which also automatically converts EPUB and other formats to the target device. You can use any email service that allow sending emails using the SMTP protocol, like [GMX](gmx.com). The following environment variables need to be defined:

* `SMTPSERVER`: The URL of the SMTP server to be used, for example `mail.gmx.com`.
* `SMTPPORT`: The port number used by the email service, defaults to `587`.
* `SMTPUSER`: The user name.
* `SMTPPASSWORD`: User's password.

### User management and access restriction

Coreander distinguish between two kinds of users: regular users and administrator users, with the latter being the only ones with the ability to create new users.

By default, Coreander allow unrestricted access to its contents, except management areas which require and administrator user. To allow access only to registered users in the whole application, pass the `REQUIRE_AUTH=true` environment variable.

### Settings

* `LIBPATH`: Absolute path to the folder containing the documents.
* `PORT`: Port number in which the webserver listens for requests. Defaults to 3000.
* `BATCHSIZE`: Number of documents persisted by the indexer in one write operation. defaults to 100.
* `COVERMAXWIDTH`: Maximum horizontal size for documents cover thumbnails in pixels. Defaults to 300.
* `SKIPREINDEX`: Whether to bypass the indexing process or not.
* `SMTPSERVER`: Address of the send mail server.
* `SMTPPORT`: Port number of the send mail server. Defaults to 587.
* `SMTPUSER`: User to authenticate against the SMTP server.
* `SMTPPASSWORD`: User's password to authenticate against the SMTP server.
* `JWT_SECRET`: String to use to sign JWTs.
* `REQUIRE_AUTH`: Require authentication to access the application if true. Defaults to false.
* `MINPASSWORDLENGTH`: minimum length acceptable for passwords. Defaults to 5.
