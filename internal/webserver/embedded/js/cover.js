"use strict"

document.querySelectorAll(".cover").forEach(function(elem) {
    if (!elem.getAttribute('data-src')) {
        return;
    }

    elem.addEventListener("error", () => {
        elem.onerror = null;
        elem.src = '/images/generic.webp';
        const coverTitleId = elem.getAttribute("data-cover-title-id");
        document.getElementById(coverTitleId).classList.remove('d-none')
    })

    elem.setAttribute('src', elem.getAttribute('data-src'));
})
