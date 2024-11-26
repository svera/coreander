"use strict";

// Control which star icon to show when highlighting / dehighlighting an item
// To use only when we want to avoid refreshing the documents list
document.body.addEventListener('htmx:afterRequest', function(evt) {
    if (!evt.detail.successful) {
        return
    }
    evt.detail.elt.parentNode.classList.add("visually-hidden");
    if (evt.detail.elt.getAttribute('hx-delete')) {
        evt.detail.elt.parentNode.nextElementSibling.classList.remove("visually-hidden");
    } else {
        evt.detail.elt.parentNode.previousElementSibling.classList.remove("visually-hidden");
    }
});
