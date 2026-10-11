"use strict"

import { importVersioned } from './asset-version.js'

let pdfCoverModule

const loadCover = async (elem) => {
    const coverTitleId = elem.getAttribute("data-cover-title-id");
    let realSrc = elem.getAttribute('data-src');
    let objectURL
    const pdfURL = elem.getAttribute('data-pdf-src')
    if (pdfURL) {
        try {
            pdfCoverModule ??= importVersioned('./pdf-cover.js')
            const { renderPDFCover } = await pdfCoverModule
            const maxWidth = Number(document.querySelector('meta[name="cover-max-width"]')?.content ?? 600)
            const blob = await renderPDFCover(pdfURL, maxWidth)
            objectURL = URL.createObjectURL(blob)
            realSrc = objectURL
        } catch (error) {
            console.error(`Could not render PDF cover for ${pdfURL}:`, error)
            document.getElementById(coverTitleId)?.classList.remove('d-none')
            return
        }
    }
    const preloader = new Image();

    preloader.addEventListener("load", () => {
        elem.src = realSrc;
        if (objectURL) {
            elem.addEventListener('load', () => URL.revokeObjectURL(objectURL), { once: true })
            elem.addEventListener('error', () => URL.revokeObjectURL(objectURL), { once: true })
        }
        elem.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 250, easing: 'ease' });
        const overlay = document.getElementById(coverTitleId)
        if (overlay) {
            overlay.remove()
        }
    })

    preloader.addEventListener("error", () => {
        if (objectURL) URL.revokeObjectURL(objectURL)
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
        if (!elem.getAttribute('data-src') && !elem.getAttribute('data-pdf-src')) {
            return;
        }

        if (elem.classList.contains('loaded')) {
            return;
        }

        elem.classList.add('loaded');
        if (elem.hasAttribute('data-cover-eager')) {
            loadCover(elem);
        } else {
            intersectionObserver.observe(elem);
        }
    })
}

document.addEventListener('DOMContentLoaded', coversLoader);
document.body.addEventListener('htmx:afterSettle', coversLoader);

const observer = new MutationObserver(coversLoader);

// Start observing the target node for configured mutations
const node = document.getElementsByTagName("body")[0];
observer.observe(node, { attributes: true, childList: false, subtree: true });
