"use strict"

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
        initShareRecipients(container, endpoint)
    })
}

initAllShareRecipients()
document.addEventListener('htmx:afterSwap', event => {
    if (event && event.target) {
        initAllShareRecipients(event.target)
    }
})

function initShareRecipients(container, endpoint) {
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
    let optionLookup = new Map()
    let activeFetchController = null
    let debounceTimer = null
    let lastFetchedQuery = ''
    let lastFetchedUsers = []

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
    }

    function fetchUsernames(query) {
        if (activeFetchController) {
            activeFetchController.abort()
        }
        activeFetchController = new AbortController()
        const url = `${endpoint}?q=${encodeURIComponent(query)}`
        return fetch(url, { signal: activeFetchController.signal })
            .then(response => {
                if (!response.ok) {
                    throw new Error('Failed to fetch usernames')
                }
                return response.json()
            })
            .then(users => {
                availableUsers = Array.isArray(users) ? users : []
                lastFetchedQuery = query.toLowerCase()
                lastFetchedUsers = availableUsers
                return availableUsers
            })
            .catch(error => {
                if (error && error.name === 'AbortError') {
                    return []
                }
                console.error('Error loading usernames:', error)
                return []
            })
    }

    function maybePopulateDatalist(value) {
        if (!value) {
            populateDatalistForValue('')
            return
        }
        const normalizedValue = value.toLowerCase()
        if (lastFetchedQuery && normalizedValue.startsWith(lastFetchedQuery)) {
            availableUsers = lastFetchedUsers
            populateDatalistForValue(value)
            return
        }
        fetchUsernames(value).then(() => populateDatalistForValue(value))
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

    function handleSelection(value) {
        if (!value || !optionLookup.has(value)) {
            return false
        }
        const normalized = optionLookup.get(value) || value
        addRecipient(normalized)
        return true
    }

    function tryAddRecipient(value) {
        const trimmed = value.trim()
        if (!trimmed) {
            return false
        }
        // If it matches a datalist option, use the normalized username
        if (optionLookup.has(trimmed)) {
            const normalized = optionLookup.get(trimmed)
            addRecipient(normalized)
            return true
        }
        // Otherwise, try to add as-is (for submit button when user typed something)
        addRecipient(trimmed)
        return true
    }

    const form = container.closest('form')
    const submitButton = form?.querySelector('.share-submit')
    if (submitButton) {
        submitButton.addEventListener('click', async event => {
            tryAddRecipient(input.value.trim())
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
            const modalEl = submitButton.closest('.modal')
            if (modalEl) {
                bootstrap.Modal.getOrCreateInstance(modalEl).hide()
            }
        })
    }

    updateBadges()

    input.addEventListener('input', e => {
        const value = e.target.value.trim()
        // Only check for exact matches on input - don't prevent datalist population
        // The datalist should populate to show suggestions as user types
        if (debounceTimer) {
            clearTimeout(debounceTimer)
        }
        debounceTimer = setTimeout(() => {
            maybePopulateDatalist(value)
        }, 200)
    })

    input.addEventListener('change', e => {
        const value = e.target.value.trim()
        if (handleSelection(value)) {
            return
        }
    })

    input.addEventListener('blur', e => {
        const value = e.target.value.trim()
        if (handleSelection(value)) {
            return
        }
    })

    input.addEventListener('keydown', e => {
        if (e.key === 'Enter') {
            e.preventDefault()
            handleSelection(input.value.trim())
        } else if (e.key === 'Backspace' && input.value === '' && selectedRecipients.length > 0) {
            removeRecipient(selectedRecipients.length - 1)
        }
    })
}

function showShareSuccess(button) {
    const messageSelector = button.getAttribute('data-success-message-selector')
    if (!messageSelector) {
        return
    }
    const messageElement = button.closest('.modal').querySelector(messageSelector)
    if (!messageElement) {
        return
    }
    const message = messageElement.innerHTML
    showToast(message, 'success')
}

function showShareError(button) {
    const message = button.getAttribute('data-error-message')
    if (message) {
        showToast(message, 'danger')
    }
}
