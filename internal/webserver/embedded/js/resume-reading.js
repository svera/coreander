"use strict"

// Refresh resume reading when the home page is restored from the back-forward cache
// (e.g. after leaving the reader via the browser back button).
window.addEventListener("pageshow", (event) => {
    if (!event.persisted) {
        return
    }
    const section = document.getElementById("resume-reading-section")
    if (section) {
        htmx.trigger(section, "refresh")
    }
})
