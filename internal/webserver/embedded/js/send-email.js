"use strict"

Array.from(document.getElementsByClassName("send-email")).forEach(form => {
    form.addEventListener("submit", event => {
        event.preventDefault();

        const submit = form.querySelector('button');
        let sendIcon = form.querySelector('.bi-send-fill');
        let spinner = form.querySelector('.spinner-border');

        submit.setAttribute("disabled", true);
        spinner.classList.remove("visually-hidden");
        sendIcon.classList.add("visually-hidden");
        fetch(form.getAttribute("action"), {
            method: "POST",
            headers: {
                'Content-Type': 'application/x-www-form-urlencoded'
            },
            body: new URLSearchParams({
                'email': form.elements[0].value
            })
        })
        .then((response) => {
            let message = form.querySelector(".send-email-message")
            message.classList.remove("visually-hidden");
            if (!response.ok) {
                if (response.status == "403") {
                    location.reload()
                } else {
                    message.innerHTML = form.getAttribute("data-error-message");
                    message.classList.remove("text-success");
                    message.classList.add("text-danger");
                }
            } else {
                message.innerHTML = form.getAttribute("data-success-message");
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
    });
});

document.body.addEventListener('htmx:responseError', function (evt) {
    const parent = evt.detail.elt.closest(".actions").parentNode;
    parent.querySelector(".quick-email-error").classList.remove("d-none");
    parent.querySelector(".quick-email-success").classList.add("d-none");
});

document.body.addEventListener('htmx:afterRequest', function (evt) {
    const post = evt.detail.elt.getAttribute("hx-post")
    if (!evt.detail.failed && post && post.includes("/send")) {
        const parent = evt.detail.elt.closest(".actions").parentNode;
        parent.querySelector(".quick-email-error").classList.add("d-none");
        parent.querySelector(".quick-email-success").classList.remove("d-none");
    }
});
