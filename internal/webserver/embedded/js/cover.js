"use strict"

const coversLoader = () => {
    document.querySelectorAll("img.cover").forEach(function(elem) {
        if (!elem.getAttribute('data-src')) {
            return;
        }

        if (elem.classList.contains('loaded')) {
            return;
        }

        const coverTitleId = elem.getAttribute("data-cover-title-id");

        elem.addEventListener("load", () => {
            if (elem.src.endsWith("/images/generic.webp")) {
                return
            }
            document.getElementById(coverTitleId).classList.add('d-none')
        })

        elem.addEventListener("error", () => {
            elem.onerror = null;
            elem.src = '/images/generic.webp';
            document.getElementById(coverTitleId).classList.remove('d-none')
        })

        elem.setAttribute('src', elem.getAttribute('data-src'));
        elem.classList.add('loaded');
    })
}

document.addEventListener('DOMContentLoaded', coversLoader());

const observer = new MutationObserver(coversLoader);

// Start observing the target node for configured mutations
const node = document.getElementsByTagName("body")[0];
observer.observe(node, { attributes: true, childList: false, subtree: true });
