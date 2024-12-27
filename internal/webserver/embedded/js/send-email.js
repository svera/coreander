"use strict"

document.body.addEventListener('htmx:responseError', function (evt) {
    if (evt.detail.xhr.status === 403) {
        location.reload()
        return
    }

    const toast = document.getElementById('live-toast')
    toast.querySelector(".toast-body").innerHTML = evt.detail.elt.getAttribute("data-error-message")
    const toastBootstrap = bootstrap.Toast.getOrCreateInstance(toast)
    toastBootstrap.show()
});

document.body.addEventListener('htmx:afterRequest', function (evt) {
    const post = evt.detail.elt.getAttribute("hx-post")
    if (!evt.detail.failed && post && post.includes("/send")) {
        const toast = document.getElementById('live-toast')
        toast.querySelector(".toast-body").innerHTML = evt.detail.elt.getAttribute("data-success-message")
        const toastBootstrap = bootstrap.Toast.getOrCreateInstance(toast)
        toastBootstrap.show()
    }
});
