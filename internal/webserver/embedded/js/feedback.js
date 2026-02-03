"use strict"

/**
 * Shows a toast notification
 * @param {string} message - The message to display (can be HTML)
 * @param {'success'|'danger'} type - The toast type
 */
function showToast(message, type = 'success') {
    const toastId = type === 'danger' ? 'live-toast-danger' : 'live-toast-success'
    const toast = document.getElementById(toastId)
    if (!toast || !message) {
        return
    }
    toast.querySelector(".toast-body").innerHTML = message
    const toastBootstrap = bootstrap.Toast.getOrCreateInstance(toast)
    toastBootstrap.show()
}

// Make showToast available globally for non-module scripts
window.showToast = showToast

htmx.on("htmx:beforeSwap", (evt) => {
    // Allow 422 and 400 responses to swap
    // We treat these as form validation errors
    if (evt.detail.xhr.status === 422 || evt.detail.xhr.status === 400) {
        evt.detail.shouldSwap = true;
        evt.detail.isError = false;
    }
})

let dataSuccessMessage, dataErrorMessage = null

htmx.on('htmx:beforeRequest', (evt) => {
    dataSuccessMessage = evt.detail.elt.getAttribute("data-success-message")
    dataErrorMessage = evt.detail.elt.getAttribute("data-error-message")
})

htmx.on('htmx:afterRequest', (evt) => {
    const toastSuccess = document.getElementById('live-toast-success')
    const toastDanger = document.getElementById('live-toast-danger')
    const unexpectedServerErrorText = document.getElementsByTagName('main')[0].dataset.unexpectedServerError

    if (!evt.detail.xhr) {
        return;
    }
    const xhr = evt.detail.xhr;

    if (evt.detail.failed) {
        // Server error with response contents, equivalent to htmx:responseError
        if (xhr.status === 403) {
            return location.reload()
        }

        if (xhr.status >= 500) {
            console.warn("Server error", evt.detail)
            showToast(unexpectedServerErrorText + `${xhr.status} - ${xhr.statusText}`, 'danger')
            return
        }

        if (dataErrorMessage !== null) {
            showToast(dataErrorMessage, 'danger')
            return
        }
    }

    if (xhr.status === 200 && dataSuccessMessage !== null && dataSuccessMessage !== undefined) {
        showToast(dataSuccessMessage, 'success')
    }

    if (xhr.status === 200) {
        const closeTarget = evt.detail.elt.getAttribute("data-close-modal")
        if (closeTarget) {
            const modalEl = document.querySelector(closeTarget)
            if (modalEl) {
                bootstrap.Modal.getOrCreateInstance(modalEl).hide()
            }
        }
    }
});

const setUpWarningsListener = () => {
    document.querySelectorAll('[data-warning-once]').forEach((el) => {
        el.addEventListener('click', () => {
            document.cookie = `warning-once=${el.getAttribute('data-warning-once')}; path=/`;
        });
    });
}

const warningsObserver = new MutationObserver(setUpWarningsListener);

// Start observing the target node for configured mutations
const node = document.getElementsByTagName('body')[0];
warningsObserver.observe(node, { attributes: true, childList: false, subtree: true });

document.addEventListener('click', (evt) => {
    const button = evt.target.closest('[data-copy-target], [data-copy-text]')
    if (!button) {
        return
    }
    evt.preventDefault()
    const targetSelector = button.getAttribute('data-copy-target')
    const directText = button.getAttribute('data-copy-text')
    let text = ''
    if (targetSelector) {
        const input = document.querySelector(targetSelector)
        if (input) {
            text = input.value || input.getAttribute('value') || ''
        }
    } else if (directText) {
        text = directText
    }
    if (text === '') {
        return
    }

    const showCopyToast = () => {
        const message = button.getAttribute('data-copy-success')
        if (message) {
            showToast(message, 'success')
        }
    }

    const fallbackCopy = () => {
        const textarea = document.createElement('textarea')
        textarea.value = text
        textarea.setAttribute('readonly', '')
        textarea.style.position = 'absolute'
        textarea.style.left = '-9999px'
        document.body.appendChild(textarea)
        textarea.select()
        try {
            document.execCommand('copy')
        } finally {
            document.body.removeChild(textarea)
        }
        showCopyToast()
    }

    if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(showCopyToast).catch(fallbackCopy)
        return
    }

    fallbackCopy()
})
