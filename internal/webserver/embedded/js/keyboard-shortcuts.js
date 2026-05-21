'use strict'

const KEYBOARD_PAGINATION_KEY = 'coreander-keyboard-pagination'
const SCROLL_STEP_PX = 40

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

function handleKeydown(event) {
    if (shouldIgnoreKeydown(event)) {
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
