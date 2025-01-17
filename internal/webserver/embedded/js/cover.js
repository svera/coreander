"use strict"

document.querySelectorAll(".cover").forEach(function(elem) {
    elem.addEventListener("error", () => {
        elem.onerror = null
        const coverTitleId = elem.getAttribute("data-cover-title-id")
        elem.parentNode.children[0].srcset = elem.src
        document.getElementById(coverTitleId).classList.remove('d-none')
    })
})
