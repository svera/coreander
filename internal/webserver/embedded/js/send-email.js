"use strict"

import { handleResponseError } from './handle-response-error.js'

document.body.addEventListener('htmx:configRequest', function (evt) {
    const post = evt.detail.elt.getAttribute("hx-post")

    if (!post || !post.includes("/send")) {
        return
    }

    let text = evt.detail.elt.getAttribute("data-success-message")
    text = text.replace("${email}", evt.detail.parameters['email'])
    evt.detail.elt.setAttribute("data-success-message", text)

    text = evt.detail.elt.getAttribute("data-error-message")
    text = text.replace("${email}", evt.detail.parameters['email'])
    evt.detail.elt.setAttribute("data-error-message", text)
})

document.body.addEventListener('htmx:responseError', function (evt) {
    const post = evt.detail.elt.getAttribute("hx-post")
    if (!post || !post.includes("/send")) {
        return
    }

    handleResponseError(evt)
})

document.body.addEventListener('htmx:afterRequest', function (evt) {
    const post = evt.detail.elt.getAttribute("hx-post")
    if (!evt.detail.failed && post && post.includes("/send")) {
        const toast = document.getElementById('live-toast-success')
        toast.querySelector(".toast-body").innerHTML = evt.detail.elt.getAttribute("data-success-message")
        const toastBootstrap = bootstrap.Toast.getOrCreateInstance(toast)
        toastBootstrap.show()
    }
})
