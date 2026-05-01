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

function escapeHtmlText(s) {
    return String(s)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;')
}

// Replace ${email} from hx-include targets (e.g. quick-email-*) without mutating data-* attributes.
function resolveEmailPlaceholder(message, elt) {
    if (!message || message.indexOf('${email}') === -1) {
        return message
    }
    const inc = elt && elt.getAttribute ? elt.getAttribute('hx-include') || '' : ''
    let email = ''
    const re = /id='([^']+)'/g
    let m
    while ((m = re.exec(inc)) !== null) {
        const field = document.getElementById(m[1])
        if (!field || field.tagName !== 'INPUT') {
            continue
        }
        const id = m[1]
        if (field.name === 'email' || field.type === 'email' || id.indexOf('quick-email') === 0) {
            email = String(field.value || '').trim()
            break
        }
    }
    return message.split('${email}').join(escapeHtmlText(email))
}

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
    const unexpectedServerErrorText = document.getElementsByTagName('main')[0].dataset.unexpectedServerError

    if (!evt.detail.xhr) {
        return;
    }
    const xhr = evt.detail.xhr;
    const elt = evt.detail.elt

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
            showToast(resolveEmailPlaceholder(dataErrorMessage, elt), 'danger')
            return
        }
    }

    if ((xhr.status === 200 || xhr.status === 204) && dataSuccessMessage) {
        showToast(resolveEmailPlaceholder(dataSuccessMessage, elt), 'success')
    }

    if (xhr.status === 200 || xhr.status === 204) {
        const closeTarget = elt.getAttribute("data-close-modal")
        if (closeTarget) {
            const modalEl = document.querySelector(closeTarget)
            if (modalEl && window.bootstrap && bootstrap.Modal) {
                bootstrap.Modal.getOrCreateInstance(modalEl).hide()
            }
            if (closeTarget === '#inviteUserModal') {
                const ta = document.getElementById('invite-email')
                if (ta) {
                    ta.value = ''
                }
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

    if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(() => {
            const message = button.getAttribute('data-copy-success')
            if (message) {
                showToast(message, 'success')
            }
        })
    }
})
