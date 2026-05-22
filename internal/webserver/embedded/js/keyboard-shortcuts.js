'use strict'

const KEYBOARD_PAGINATION_KEY = 'coreander-keyboard-pagination'
const SCROLL_STEP_PX = 40
const GO_SEQUENCE_MS = 2000
const GO_DESTINATIONS = {
    c: '/completed',
    h: '/highlights',
    u: '/upload',
}

let pendingGoKey = false
let pendingGoTimeout = null

function shouldIgnoreKeydown(event) {
    const target = event.target
    if (!target || !(target instanceof Element)) {
        return true
    }
    if (event.ctrlKey || event.metaKey || event.altKey) {
        return true
    }
    const tag = target.tagName
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') {
        return true
    }
    if (target.isContentEditable) {
        return true
    }
    if (document.querySelector('.modal.show, .offcanvas.show')) {
        return true
    }
    return false
}

function isVisible(element) {
    return element && !element.disabled && element.getClientRects().length > 0
}

function getSearchInput() {
    const candidates = [
        document.querySelector('#searchbox-container input[name="search"]'),
        document.querySelector('#sidebar-search'),
        ...document.querySelectorAll('#searchbox'),
        document.querySelector('#searchbox-offcanvas'),
    ]

    for (const input of candidates) {
        if (isVisible(input)) {
            return input
        }
    }

    return [...document.querySelectorAll('input[type="search"][name="search"]')].find(isVisible) ?? null
}

function getPaginationLink(rel) {
    return document.querySelector(`nav[data-pagination-nav] a[rel="${rel}"]`)
}

function blurSearchInputs() {
    document.querySelectorAll('input[type="search"][name="search"]').forEach(input => {
        input.blur()
    })
}

function shouldIgnoreShortcutsHelp(event) {
    const target = event.target
    if (!target || !(target instanceof Element)) {
        return true
    }
    if (event.ctrlKey || event.metaKey || event.altKey) {
        return true
    }
    const tag = target.tagName
    if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') {
        return true
    }
    return !!target.isContentEditable
}

function clearGoSequence() {
    pendingGoKey = false
    if (pendingGoTimeout !== null) {
        clearTimeout(pendingGoTimeout)
        pendingGoTimeout = null
    }
}

function startGoSequence() {
    clearGoSequence()
    pendingGoKey = true
    pendingGoTimeout = setTimeout(clearGoSequence, GO_SEQUENCE_MS)
}

function letterKey(event) {
    return event.key.length === 1 ? event.key.toLowerCase() : ''
}

function handleGoSequence(event) {
    const key = letterKey(event)
    if (!key) {
        return false
    }

    if (pendingGoKey) {
        clearGoSequence()
        const destination = GO_DESTINATIONS[key]
        if (destination) {
            event.preventDefault()
            window.location.assign(destination)
            return true
        }
        return false
    }

    if (key === 'g') {
        event.preventDefault()
        startGoSequence()
        return true
    }

    return false
}

function toggleShortcutsModal(event) {
    const modalEl = document.getElementById('keyboard-shortcuts-modal')
    if (!modalEl || typeof bootstrap === 'undefined') {
        return
    }
    event.preventDefault()
    const modal = bootstrap.Modal.getOrCreateInstance(modalEl)
    if (modalEl.classList.contains('show')) {
        modal.hide()
    } else {
        modal.show()
    }
}

function handleKeydown(event) {
    if (event.key === '?') {
        if (!shouldIgnoreShortcutsHelp(event)) {
            toggleShortcutsModal(event)
        }
        return
    }

    if (shouldIgnoreKeydown(event)) {
        return
    }

    if (handleGoSequence(event)) {
        return
    }

    if (event.key === '/') {
        const input = getSearchInput()
        if (input) {
            event.preventDefault()
            input.focus({ preventScroll: false })
            input.select()
        }
        return
    }

    if (event.key === 'ArrowUp' || event.key === 'ArrowDown') {
        const delta = event.key === 'ArrowUp' ? -SCROLL_STEP_PX : SCROLL_STEP_PX
        document.body.scrollBy({ top: delta, left: 0, behavior: 'auto' })
        event.preventDefault()
        return
    }

    let link = null
    if (event.key === 'ArrowLeft') {
        link = getPaginationLink('prev')
    } else if (event.key === 'ArrowRight') {
        link = getPaginationLink('next')
    } else {
        return
    }

    if (!link) {
        return
    }

    event.preventDefault()
    sessionStorage.setItem(KEYBOARD_PAGINATION_KEY, '1')
    window.location.assign(link.href)
}

document.addEventListener('keydown', handleKeydown)

function restoreAfterKeyboardPagination() {
    if (sessionStorage.getItem(KEYBOARD_PAGINATION_KEY) !== '1') {
        return
    }
    sessionStorage.removeItem(KEYBOARD_PAGINATION_KEY)
    blurSearchInputs()
    requestAnimationFrame(() => {
        blurSearchInputs()
        setTimeout(blurSearchInputs, 0)
    })
}

document.addEventListener('pageshow', restoreAfterKeyboardPagination)
document.addEventListener('DOMContentLoaded', restoreAfterKeyboardPagination)
