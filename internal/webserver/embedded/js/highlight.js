"use strict";

let links = document.querySelectorAll("a.highlight, a.dehighlight");

Array.from(links).forEach((link) => {
    link.addEventListener("click", (event) => {
        event.preventDefault();
        const parent = link.closest(".actions");
        const method = link.getAttribute("data-method");

        let highlightLinkParent = parent.querySelector(".highlight");
        let dehighlightLinkParent = parent.querySelector(".dehighlight");
        fetch(link.getAttribute("href"), {
            method: method,
            headers: {
                "Content-Type": "application/x-www-form-urlencoded",
            },
            credentials: "same-origin",
        })
        .then((response) => {
            if (response.ok) {
                if (method == "DELETE") {
                    if (link.getAttribute("data-dehighlight") == "remove") {
                        location.reload();
                        return;
                    }
                    dehighlightLinkParent.classList.add("visually-hidden");
                    highlightLinkParent.classList.remove("visually-hidden");
                } else {
                    highlightLinkParent.classList.add("visually-hidden");
                    dehighlightLinkParent.classList.remove(
                        "visually-hidden"
                    );
                }
                return;
            }
            if (response.status == "403") {
                location.reload()
            }
        })
        .catch(function (error) {
            // Catch errors
            console.log(error);
        });
    });
});
