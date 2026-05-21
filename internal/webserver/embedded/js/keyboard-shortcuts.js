'use strict'

const PREV_KEYS = new Set(['ArrowLeft'])
const NEXT_KEYS = new Set(['ArrowRight'])
const SCROLL_KEYS = new Set(['ArrowUp', 'ArrowDown'])
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
    if (document.querySelector('.modal.show')) {
        return true
    }
    if (document.querySelector('.offcanvas.show')) {
        return true
    }
    return false
}

function isVisible(element) {
    if (!element) {
        return false
    }
    if (element.disabled) {
        return false
    }
    return element.getClientRects().length > 0
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

function focusSearch() {
    const input = getSearchInput()
    if (!input) {
        return false
    }
    input.focus({ preventScroll: false })
    input.select()
    return true
}

function getPaginationLink(direction) {
    const link = document.querySelector(
        `nav[data-pagination-nav] a[data-pagination="${direction}"]`
    )
    if (!link) {
        return null
    }
    if (link.closest('.page-item')?.classList.contains('disabled')) {
        return null
    }
    if (link.getAttribute('aria-disabled') === 'true') {
        return null
    }
    const href = link.getAttribute('href')
    if (!href) {
        return null
    }
    return link
}

function navigatePagination(direction) {
    const link = getPaginationLink(direction)
    if (!link) {
        return false
    }
    sessionStorage.setItem(KEYBOARD_PAGINATION_KEY, '1')
    document.activeElement?.blur()
    window.location.assign(link.href)
    return true
}

function blurSearchInputs() {
    document.querySelectorAll('input[type="search"][name="search"]').forEach(input => {
        input.blur()
    })
}

function focusScrollContainer() {
    const main = document.querySelector('main')
    if (!main) {
        return
    }
    main.classList.add('keyboard-scroll-anchor')
    if (!main.hasAttribute('tabindex')) {
        main.setAttribute('tabindex', '-1')
    }
    main.focus({ preventScroll: true })
}

function releaseFocusForScroll() {
    blurSearchInputs()
    focusScrollContainer()
}

function restoreFocusAfterKeyboardPagination() {
    if (sessionStorage.getItem(KEYBOARD_PAGINATION_KEY) !== '1') {
        return
    }
    sessionStorage.removeItem(KEYBOARD_PAGINATION_KEY)

    releaseFocusForScroll()
    requestAnimationFrame(() => {
        releaseFocusForScroll()
        setTimeout(releaseFocusForScroll, 0)
    })
}

function scrollPageVertically(event) {
    const delta = event.key === 'ArrowUp' ? -SCROLL_STEP_PX : SCROLL_STEP_PX
    document.body.scrollBy({ top: delta, left: 0, behavior: 'auto' })
    event.preventDefault()
}

function handleKeydown(event) {
    if (shouldIgnoreKeydown(event)) {
        return
    }

    if (event.key === '/') {
        if (focusSearch()) {
            event.preventDefault()
        }
        return
    }

    if (SCROLL_KEYS.has(event.key)) {
        scrollPageVertically(event)
        return
    }

    if (!document.querySelector('nav[data-pagination-nav]')) {
        return
    }

    let direction = null
    if (PREV_KEYS.has(event.key)) {
        direction = 'prev'
    } else if (NEXT_KEYS.has(event.key)) {
        direction = 'next'
    } else {
        return
    }

    if (navigatePagination(direction)) {
        event.preventDefault()
    }
}

document.addEventListener('keydown', handleKeydown)
document.addEventListener('DOMContentLoaded', restoreFocusAfterKeyboardPagination)
document.addEventListener('pageshow', restoreFocusAfterKeyboardPagination)
