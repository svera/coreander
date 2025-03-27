"use strict"

document.querySelectorAll(".cover").forEach(function(elem) {
    if (!elem.getAttribute('data-src')) {
        return;
    }

    elem.setAttribute('src', elem.getAttribute('data-src'));

    elem.addEventListener("error", () => {
        const coverTitleId = elem.getAttribute("data-cover-title-id")
        elem.onerror = null;
        elem.src = '/images/generic.jpg';
        document.getElementById(coverTitleId).classList.remove('d-none')
    })
})
