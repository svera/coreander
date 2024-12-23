# ![Coreander logo](readme-header.png)

A personal documents server, Coreander indexes the documents (EPUBs and PDFs with no DRM) that it finds in the passed folder, and provides a web interface to search and access them.

![Coreander screenshot](screenshot.png)

## Features
* Single binary with all dependencies included.
* Fast search engine powered by [Bleve](https://github.com/blevesearch/bleve), with support for documents in multiple languages.
* Search by author, title and even document series ([Calibre's](https://calibre-ebook.com/) `series` meta supported)
* Improved search for documents with metadata in english, spanish, french, italian, german and portuguese, including genre and singular/plural forms of words in the results among others.
* Estimated reading time calculation.
* High-performance web server powered by [Fiber](https://github.com/gofiber/fiber).
* Lightweight, responsive web interface based on [Bootstrap](https://getbootstrap.com/).
* Web interface available in english, spanish and french, more languages can be easily added.
* New documents added or removed to/from the library folder are automatically indexed (Linux only).
* [Send to email supported](#send-to-email).
* Read indexed epubs and PDFs from Coreander's interface thanks to [foliate-js](https://github.com/johnfactotum/foliate-js).
* Restrictable access only to registered users.
* Upload documents through the web interface.
* Download as kepub (epub for Kobo devices) converted on the fly thanks to [Kepubify](https://github.com/pgaskin/kepubify).

## Installation

Binaries for Windows 64 bit, Linux 32 and 64 bits for both X86 and ARM (Raspberry Pi and other SBCs) and Macs with Intel and Apple processors are available at [releases](https://github.com/svera/coreander/releases/latest). Just download and unzip the one appropriate for your system.

### Building from source
Coreander's only requirement is Go 1.21.

There are two possibilities for building Coreander from source:
* If you have [Mage](https://magefile.org) installed in your system, just type `mage install` from the source code folder.
* Otherwise, a simple `go build` or `go install` will do, although no version information will be added to the executable.

## How to use
Coreander is designed to be run as a service managed by [systemd](https://systemd.io) or any other service manager. For example, in Raspberry Pi OS, just create a file called `/etc/systemd/system/coreander.service` with the following contents:

```
[Unit]
Description=coreander

[Service]
Restart=always
RestartSec=5s
WorkingDirectory=<absolute path to directory which contains coreander binary>
ExecStart=<absolute path to coreander binary>
PermissionsStartOnly=true
SyslogIdentifier=sleepservice
User=<user which will execute this service>
Environment="LIB_PATH=<absolute path to the library>"

```

then, start the service with `service coreander start`. You can manage it with the usual commands `start`, `stop` and `status`. Refer to your service manager documentation for more information.

Coreander requires a `LIB_PATH` environment variable to be set, which tells the application where your documents are located.

On first run, Coreander will index the documents in your library, creating a database with those entries located at `$home/coreander/index`. Depending on your system's performance and the size of your library this may take a while. Also, the database can grow fairly big, so make sure you have enough free space on disk.

Every time is run, the application scans the library folder only for documents not yet indexed and adds them to the index. You can force to index all documents wether they were previously indexed or not by setting the environment variable `FORCE_INDEXING` to `true`.

Even if the application is still indexing entries, you can access its web interface right away. Just open a web browser and go to `localhost:3000` (replace `localhost` with the hostname / IP address of the machine where the server is running if you want to access it from another system). It is possible to change the listening port just executing the application with the `PORT` environment variable (e. g. `PORT=4000 coreander`)

### Email

Some features rely on having an SMTP email service set up, and won't be available otherwise:

* Send document to email.
* Recover user password.

You can use any email service that allow sending emails using the SMTP protocol, like [GMX](https://gmx.com/mail). The following environment variables need to be defined:

* `SMTP_SERVER`: The URL of the SMTP server to be used, for example `mail.gmx.com`.
* `SMTP_PORT`: The port number used by the email service, defaults to `587`.
* `SMTP_USER`: The user name.
* `SMTP_PASSWORD`: User's password.

#### Send to email

Coreander can send documents through email. This way, you can take advantage of services such as [Amazon's send to email](https://www.amazon.com/gp/help/customer/display.html?nodeId=G7NECT4B4ZWHQ8WV), which also automatically converts EPUB and other formats to the target device.

### User management and access restriction

Coreander distinguish between two kinds of users: regular users and administrator users, with the latter being the only ones with the ability to create new users and upload and delete documents.

By default, Coreander allow unrestricted access to its contents, except management areas which require an administrator user. To allow access only to registered users in the whole application, pass the `REQUIRE_AUTH=true` environment variable.

On first run, Coreander creates an admin user with the following credentials:

* Email: `admin@example.com`
* Password: `admin`

**For security reasons, it is strongly encouraged to add a new admin and remove the default one as soon as possible.**

### Settings

* `LIB_PATH`: Absolute path to the folder containing the documents.
* `PORT`: Port number in which the webserver listens for requests. Defaults to 3000.
* `BATCH_SIZE`: Number of documents persisted by the indexer in one write operation. Defaults to 100.
* `COVER_MAX_WIDTH`: Maximum horizontal size for documents cover thumbnails in pixels. Defaults to 600.
* `FORCE_INDEXING`: Whether to force indexing already indexed documents or not. Defaults to false.
* `SMTP_SERVER`: Address of the send mail server.
* `SMTP_PORT`: Port number of the send mail server. Defaults to 587.
* `SMTP_USER`: User to authenticate against the SMTP server.
* `SMTP_PASSWORD`: User's password to authenticate against the SMTP server.
* `JWT_SECRET`: String to use to sign JWTs.
* `REQUIRE_AUTH`: Require authentication to access the application if true. Defaults to false.
* `MIN_PASSWORD_LENGTH`: Minimum length acceptable for passwords. Defaults to 5.
* `WORDS_PER_MINUTE`: Defines a default words per minute reading speed that will be used for not logged-in users. Defaults to 250.
* `SESSION_TIMEOUT`: Specifies the maximum time a user session may last, in hours. Floating-point values are allowed. Defaults to 24 hours.
* `RECOVERY_TIMEOUT`: Specifies the maximum time a user recovery link may last, in hours. Floating-point values are allowed. Defaults to 2 hours.
* `UPLOAD_DOCUMENT_MAX_SIZE`: Maximum document size allowed to be uploaded to the library, in megabytes. Set this to 0 to unlimit upload size. Defaults to 20 megabytes.
* `FQDN`: Domain name of the server. If Coreander is listening to a non-standard HTTP / HTTPS port, include it using a colon (e. g. example.com:3000). Defaults to `localhost`.
