function send(el, successMessage, errorMessage) {
    event.preventDefault();
    let form = el.closest(".send-email")

    let submit = form.querySelector('button');
    let sendIcon = document.querySelector('.bi-send-fill');
    let spinner = document.querySelector('.spinner-border');

    submit.setAttribute("disabled", true);
    spinner.classList.remove("visually-hidden");
    sendIcon.classList.add("visually-hidden");
    fetch('/send', {
        method: "POST",
        headers: {
            'Content-Type': 'application/x-www-form-urlencoded'
        },
        body: new URLSearchParams({
            'email': form.elements[0].value,
            'slug': form.elements[1].value,        
        })
    })
    .then((response) => {
        message = el.querySelector(".send-email-message")
        message.classList.remove("visually-hidden");
        if (!response.ok) {
            message.innerHTML = errorMessage;
            message.classList.remove("text-success");
            message.classList.add("text-danger");
        } else {
            message.innerHTML = successMessage;
            message.classList.remove("text-danger");
            message.classList.add("text-success");
        }
        submit.removeAttribute("disabled");
        sendIcon.classList.remove("visually-hidden");
        spinner.classList.add("visually-hidden");
    })
    .catch(function (error) {
        // Catch errors
        console.log(error);
    });
}

function highlightToggle(slug, el, method, onDehighlight = "") {
    event.preventDefault();
    let parent = el.closest(".actions")

    let highlightLinkParent = parent.querySelector(".highlight");
    let dehighlightLinkParent = parent.querySelector(".dehighlight");
    fetch(
        el.getAttribute("href"), {
            method: method,
            headers: {
                'Content-Type': 'application/x-www-form-urlencoded'
            },
            credentials: "same-origin",
            body: new URLSearchParams({
                'slug': slug,        
            })
        }
    )
    .then((response) => {
        if (response.ok) {
            if (method == "DELETE") {
                if (onDehighlight == "remove") {
                    location.reload();
                    return;
                }
                dehighlightLinkParent.classList.add("visually-hidden");
                highlightLinkParent.classList.remove("visually-hidden");
            } else {
                highlightLinkParent.classList.add("visually-hidden");
                dehighlightLinkParent.classList.remove("visually-hidden");    
            }
            return;
        }
        console.log(response.body)
    })
    .catch(function (error) {
        // Catch errors
        console.log(error);
    });
}
