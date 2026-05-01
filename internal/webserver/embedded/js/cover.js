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
            if (elem.src.includes("/images/generic.webp")) {
                return
            }
            const overlay = document.getElementById(coverTitleId)
            if (overlay) {
                overlay.remove()
            }
        })

        elem.addEventListener("error", () => {
            elem.onerror = null;
            elem.src = '/images/generic.webp';
            const overlayOnError = document.getElementById(coverTitleId)
            if (overlayOnError) {
                overlayOnError.classList.remove('d-none')
            }
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
