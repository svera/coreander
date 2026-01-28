"use strict"

const usernamesCache = new Map()
const usernamesPromises = new Map()

function initAllShareRecipients(root = document) {
    const containers = Array.from(root.querySelectorAll('[data-share-recipients]'))
    if (containers.length === 0) {
        return
    }
    containers.forEach(container => {
        if (container.dataset.shareRecipientsInitialized === 'true') {
            return
        }
        container.dataset.shareRecipientsInitialized = 'true'
        const endpoint = container.dataset.usersEndpoint || '/users/share-recipients'
        initShareRecipients(container, endpoint, usernamesCache, usernamesPromises)
    })
}

initAllShareRecipients()
document.addEventListener('htmx:afterSwap', event => {
    if (event && event.target) {
        initAllShareRecipients(event.target)
    }
})

function initShareRecipients(container, endpoint, usernamesCache, usernamesPromises) {
    const input = container.querySelector('.share-recipients-input')
    const hiddenInput = container.querySelector('input[type="hidden"][name="recipients"]')
    let datalist = container.querySelector('datalist')
    const badgesContainer = container.querySelector('.share-recipients-badges')
    if (!input || !hiddenInput || !datalist || !badgesContainer) {
        return
    }

    const removeLabelTemplate = container.dataset.removeLabel || 'Remove recipient: %s'
    let selectedRecipients = []
    let lastInputValue = ''
    let availableUsers = []
    let isRefreshingDatalist = false
    let optionLookup = new Map()
    let datalistLoaded = false

    function populateDatalist(values) {
        const listId = datalist.id
        const newDatalist = document.createElement('datalist')
        newDatalist.id = listId
        values.forEach(value => {
            const option = document.createElement('option')
            option.value = value
            newDatalist.appendChild(option)
        })
        datalist.replaceWith(newDatalist)
        datalist = newDatalist
        refreshDatalistDisplay()
    }

    function populateDatalistForValue(value) {
        const query = value.trim().toLowerCase()
        if (!query) {
            optionLookup = new Map()
            datalist.innerHTML = ''
            refreshDatalistDisplay()
            return
        }
        const matches = availableUsers
            .map(user => buildOptionsForUser(user))
            .filter(option => option.value)
            .filter(option => {
                const keys = option.keys || []
                return keys.some(key => key.startsWith(query))
            })

        populateDatalistFromOptions(matches)
    }

    function populateDatalistFromOptions(options) {
        optionLookup = new Map()
        const values = options.map(option => {
            optionLookup.set(option.value, option.username)
            return option.value
        })
        populateDatalist(values)
        datalistLoaded = true
    }

    function buildOptionsForUser(user) {
        if (!user || !user.username || !user.name) {
            return {
                value: '',
                username: '',
                keys: [],
            }
        }
        const username = String(user.username)
        const name = String(user.name)
        return {
            value: `${name} (${username})`,
            username: username,
            keys: [username.toLowerCase(), name.toLowerCase()],
        }
    }

    function refreshDatalistDisplay() {
        const listId = datalist.id
        if (!listId) {
            return
        }
        input.setAttribute('list', '')
        requestAnimationFrame(() => {
            input.setAttribute('list', listId)
            if (typeof input.showPicker === 'function' && input.value.trim().length >= 1) {
                input.showPicker()
            }
        })
        if (input.value.trim().length >= 1 && !isRefreshingDatalist) {
            isRefreshingDatalist = true
            setTimeout(() => {
                const currentValue = input.value
                input.focus()
                input.value = currentValue
                input.setSelectionRange(currentValue.length, currentValue.length)
                input.dispatchEvent(new Event('input', { bubbles: true }))
                input.dispatchEvent(new Event('keyup', { bubbles: true }))
                isRefreshingDatalist = false
            }, 0)
        }
    }

    function preloadUsernames() {
        if (usernamesCache.has(endpoint)) {
            availableUsers = usernamesCache.get(endpoint)
            return Promise.resolve(availableUsers)
        }
        if (usernamesPromises.has(endpoint)) {
            return usernamesPromises.get(endpoint)
        }
        const promise = fetch(endpoint)
            .then(response => {
                if (!response.ok) {
                    throw new Error('Failed to fetch usernames')
                }
                return response.json()
            })
            .then(users => {
                usernamesCache.set(endpoint, users)
                availableUsers = users
                return users
            })
            .catch(error => {
                console.error('Error loading usernames:', error)
                return []
            })
        usernamesPromises.set(endpoint, promise)
        return promise
    }

    function ensureUsernamesLoaded() {
        return preloadUsernames().then(() => {
            return availableUsers
        })
    }

    function maybePopulateDatalist(value) {
        if (!value) {
            populateDatalistForValue('')
            datalistLoaded = false
            return
        }
        if (value.length === 1 && !datalistLoaded && !isRefreshingDatalist) {
            ensureUsernamesLoaded().then(() => populateDatalistForValue(input.value.trim()))
            return
        }
        // Do not reload datalist after first character.
    }

    function updateBadges() {
        badgesContainer.innerHTML = ''

        if (selectedRecipients.length === 0) {
            badgesContainer.classList.add('d-none')
            hiddenInput.value = ''
            input.required = true
            return
        }

        badgesContainer.classList.remove('d-none')
        input.required = false

        selectedRecipients.forEach((recipient, index) => {
            const badge = document.createElement('span')
            badge.className = 'badge rounded-pill text-bg-primary d-inline-flex align-items-center'
            badge.style.pointerEvents = 'all'
            badge.textContent = recipient

            const closeBtn = document.createElement('button')
            closeBtn.type = 'button'
            closeBtn.className = 'btn-close btn-close-white ms-1 mt-0 small'
            closeBtn.setAttribute('aria-label', removeLabelTemplate.replace('%s', recipient))
            closeBtn.addEventListener('click', e => {
                e.preventDefault()
                e.stopPropagation()
                removeRecipient(index)
            })
            badge.appendChild(closeBtn)

            badgesContainer.appendChild(badge)
        })

        hiddenInput.value = selectedRecipients.join(',')
    }

    function addRecipient(value) {
        const trimmed = value.trim()
        if (!trimmed) {
            return
        }

        const isDuplicate = selectedRecipients.some(existing => existing.toLowerCase() === trimmed.toLowerCase())
        if (!isDuplicate) {
            selectedRecipients.push(trimmed)
            updateBadges()
        }
        input.value = ''
    }

    function removeRecipient(index) {
        selectedRecipients.splice(index, 1)
        updateBadges()
        input.focus()
    }

    function matchesDatalistOption(value) {
        return optionLookup.has(value)
    }

    function handlePotentialMatch(value) {
        if (!value) {
            return
        }
        if (matchesDatalistOption(value)) {
            const normalized = optionLookup.get(value) || value
            addRecipient(normalized)
        }
    }

    function maybeHandleSelection(value) {
        if (!value) {
            return false
        }
        if (!matchesDatalistOption(value)) {
            return false
        }
        handlePotentialMatch(value)
        return true
    }

    const form = container.closest('form')
    const submitButton = form ? form.querySelector('.share-submit') : null
    if (submitButton) {
        submitButton.addEventListener('click', async event => {
            handlePotentialMatch(input.value.trim())
            if (!hiddenInput.value) {
                input.required = true
                input.reportValidity()
                event.preventDefault()
                event.stopPropagation()
                return
            }

            event.preventDefault()
            event.stopPropagation()

            const shareUrl = submitButton.dataset.shareUrl
            if (!shareUrl) {
                return
            }

            submitButton.setAttribute('disabled', 'disabled')

            const response = await fetch(shareUrl, {
                method: 'POST',
                body: new FormData(form),
                credentials: 'same-origin',
            })

            if (response.status === 403) {
                window.location.reload()
                return
            }

            if (!response.ok) {
                showShareError(submitButton)
                submitButton.removeAttribute('disabled')
                return
            }

            showShareSuccess(submitButton)
            submitButton.removeAttribute('disabled')
            closeShareModal(submitButton)
        })
    }

    updateBadges()

    input.addEventListener('input', e => {
        const value = e.target.value.trim()
        lastInputValue = value
        if (maybeHandleSelection(value)) {
            return
        }
        maybePopulateDatalist(value)
    })

    input.addEventListener('change', e => {
        const value = e.target.value.trim()
        if (maybeHandleSelection(value)) {
            return
        }
        maybePopulateDatalist(value)
    })

    input.addEventListener('blur', e => {
        const value = e.target.value.trim()
        if (maybeHandleSelection(value)) {
            return
        }
        maybePopulateDatalist(value)
    })

    input.addEventListener('keydown', e => {
        if (e.key === 'Enter') {
            e.preventDefault()
            maybeHandleSelection(input.value.trim())
        } else if (e.key === 'Backspace' && input.value === '' && selectedRecipients.length > 0) {
            removeRecipient(selectedRecipients.length - 1)
        }
    })
}

function showShareSuccess(button) {
    const toastSuccess = document.getElementById('live-toast-success')
    const message = button.getAttribute('data-success-message')
    if (!toastSuccess || !message) {
        return
    }
    toastSuccess.querySelector(".toast-body").innerHTML = message
    const toastBootstrap = bootstrap.Toast.getOrCreateInstance(toastSuccess)
    toastBootstrap.show()
}

function showShareError(button) {
    const toastDanger = document.getElementById('live-toast-danger')
    const message = button.getAttribute('data-error-message')
    if (!toastDanger || !message) {
        return
    }
    toastDanger.querySelector(".toast-body").innerHTML = message
    const toastBootstrap = bootstrap.Toast.getOrCreateInstance(toastDanger)
    toastBootstrap.show()
}

function closeShareModal(button) {
    const closeTarget = button.getAttribute('data-close-modal')
    if (!closeTarget) {
        return
    }
    const modalEl = document.querySelector(closeTarget)
    if (!modalEl) {
        return
    }
    bootstrap.Modal.getOrCreateInstance(modalEl).hide()
}
