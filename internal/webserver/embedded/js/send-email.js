"use strict"

let forms = document.getElementsByClassName("send-email");

Array.from(forms).forEach(form => {
    form.addEventListener("submit", event => {
        event.preventDefault();

        const submit = form.querySelector('button');
        let sendIcon = form.querySelector('.bi-send-fill');
        let spinner = form.querySelector('.spinner-border');

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
