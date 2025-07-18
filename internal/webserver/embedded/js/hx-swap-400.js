"use strict"

htmx.on("htmx:beforeSwap", (e) => {
    // Allow 422 and 400 responses to swap
    // We treat these as form validation errors
    if (e.detail.xhr.status === 422 || e.detail.xhr.status === 400) {
        e.detail.shouldSwap = true;
        e.detail.isError = false;
    }
})
