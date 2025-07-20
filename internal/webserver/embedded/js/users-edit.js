"use strict"

import { handleResponseError } from './handle-response-error.js'

let successMessage
document.body.addEventListener('htmx:configRequest', function (evt) {
    successMessage = evt.detail.elt.getAttribute("data-success-message")
    if (!evt.detail.elt.getAttribute("hx-put")) {
        return
    }
})

document.body.addEventListener('htmx:responseError', function (evt) {
    if (!evt.detail.elt.getAttribute("hx-put")) {
        return
    }
    handleResponseError(evt)
})

document.body.addEventListener('htmx:afterRequest', function (evt) {
    if (evt.detail.xhr.status === 200 && evt.detail.requestConfig.verb === "put") {
        const toast = document.getElementById('live-toast-success')
        toast.querySelector(".toast-body").innerHTML = successMessage
        const toastBootstrap = bootstrap.Toast.getOrCreateInstance(toast)
        toastBootstrap.show()
    }
})
