"use strict"

const loadCover = (elem) => {
    const coverTitleId = elem.getAttribute("data-cover-title-id");
    const realSrc = elem.getAttribute('data-src');
    const preloader = new Image();

    preloader.addEventListener("load", () => {
        elem.src = realSrc;
        elem.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 250, easing: 'ease' });
        const overlay = document.getElementById(coverTitleId)
        if (overlay) {
            overlay.remove()
        }
    })

    preloader.addEventListener("error", () => {
        const overlayOnError = document.getElementById(coverTitleId)
        if (overlayOnError) {
            overlayOnError.classList.remove('d-none')
        }
    })

    preloader.src = realSrc;
}

// Only fetch the real cover once its placeholder is about to be visible, so the
// generic cover always has a chance to render first and off-screen covers stay lazy.
const intersectionObserver = new IntersectionObserver((entries, observer) => {
    entries.forEach((entry) => {
        if (!entry.isIntersecting) {
            return;
        }
        observer.unobserve(entry.target);
        loadCover(entry.target);
    })
}, { rootMargin: '200px 0px' });

const coversLoader = () => {
    document.querySelectorAll("img.cover").forEach(function(elem) {
        if (!elem.getAttribute('data-src')) {
            return;
        }

        if (elem.classList.contains('loaded')) {
            return;
        }

        elem.classList.add('loaded');
        intersectionObserver.observe(elem);
    })
}

document.addEventListener('DOMContentLoaded', coversLoader);
document.body.addEventListener('htmx:afterSettle', coversLoader);

const observer = new MutationObserver(coversLoader);

// Start observing the target node for configured mutations
const node = document.getElementsByTagName("body")[0];
observer.observe(node, { attributes: true, childList: false, subtree: true });
