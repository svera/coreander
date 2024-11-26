"use strict"

document.querySelectorAll(".cover").forEach(function(elem) {
    elem.addEventListener("error", () => {
        elem.onerror = null
        elem.parentNode.children[0].srcset = elem.src
        elem.parentNode.parentNode.children[1].classList.remove('d-none')
    })
})
