"use strict";

document.body.addEventListener('htmx:afterRequest', function(evt) {
    if (!evt.detail.successful) {
        return
    }
    console.log(evt.detail.elt.parentNode)
    evt.detail.elt.parentNode.classList.add("visually-hidden");
    if (evt.detail.elt.getAttribute('hx-delete')) {
        evt.detail.elt.parentNode.nextElementSibling.classList.remove("visually-hidden");
    } else {
        evt.detail.elt.parentNode.previousElementSibling.classList.remove("visually-hidden");
    }
});