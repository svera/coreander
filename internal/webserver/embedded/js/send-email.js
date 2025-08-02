"use strict"

htmx.on('htmx:configRequest', (evt) => {
    let text = evt.detail.elt.getAttribute("data-success-message")
    text = text.replace("${email}", evt.detail.parameters['email'])
    evt.detail.elt.setAttribute("data-success-message", text)

    text = evt.detail.elt.getAttribute("data-error-message")
    text = text.replace("${email}", evt.detail.parameters['email'])
    evt.detail.elt.setAttribute("data-error-message", text)
})

