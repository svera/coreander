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
    const maxRecipients = parseInt(container.dataset.maxRecipients || '10', 10)
    const maxRecipientsErrorTemplate = container.dataset.maxRecipientsError || `Maximum ${maxRecipients} recipients allowed`
    
    // Store original placeholder
    const originalPlaceholder = input.placeholder || 'Add usernames...'
    input.setAttribute('data-original-placeholder', originalPlaceholder)
    
    let selectedRecipients = []
    let lastInputValue = ''
    let availableUsers = []
    let optionLookup = new Map()
    let activeFetchController = null
    let debounceTimer = null
    let lastFetchedQuery = ''
    let lastFetchedUsers = []
    let isAddingRecipient = false // Lock to prevent concurrent additions
    
    // Getter function that always returns trimmed array (defensive)
    // Note: Does NOT call updateBadges to avoid circular dependency
    function getSelectedRecipients() {
        if (selectedRecipients.length > maxRecipients) {
            selectedRecipients = selectedRecipients.slice(0, maxRecipients)
        }
        return selectedRecipients
    }
    
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

    function isAtLimit() {
        return selectedRecipients.length >= maxRecipients
    }

    function updateBadges() {
        badgesContainer.innerHTML = ''

        const currentLength = selectedRecipients.length
        const recipients = getSelectedRecipients()

        if (currentLength === 0) {
            badgesContainer.classList.add('d-none')
            hiddenInput.value = ''
            input.required = true
            input.disabled = false
            input.readOnly = false
            input.removeAttribute('aria-disabled')
            input.style.cursor = ''
            input.style.opacity = ''
            input.placeholder = originalPlaceholder
            return
        }

        badgesContainer.classList.remove('d-none')
        input.required = false
        
        // Disable input when at limit to prevent adding more
        const atLimit = currentLength >= maxRecipients
        if (atLimit) {
            input.disabled = true
            input.readOnly = true
            input.placeholder = maxRecipientsErrorTemplate
            input.setAttribute('aria-disabled', 'true')
            input.style.cursor = 'not-allowed'
            input.style.opacity = '0.6'
        } else {
            input.disabled = false
            input.readOnly = false
            input.placeholder = originalPlaceholder
            input.removeAttribute('aria-disabled')
            input.style.cursor = ''
            input.style.opacity = ''
        }

        // Render badges - CRITICAL: Only render up to maxRecipients
        // Use the getter which ensures we never exceed the limit
        recipients.forEach((recipient, index) => {
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

        // Use getter to ensure we have the correct array (already trimmed)
        hiddenInput.value = getSelectedRecipients().join(',')
    }

    function addRecipient(value) {
        // Prevent concurrent additions
        if (isAddingRecipient) {
            return false
        }
        
        const trimmed = value.trim()
        if (!trimmed) {
            return false
        }

        // Check limit FIRST before any other checks
        if (isAtLimit()) {
            input.value = ''
            return false
        }

        const recipients = getSelectedRecipients()
        const isDuplicate = recipients.some(existing => existing.toLowerCase() === trimmed.toLowerCase())
        if (isDuplicate) {
            input.value = ''
            return false
        }

        // Set lock
        isAddingRecipient = true
        
        try {
            // One more check after lock is set - CRITICAL CHECK
            if (isAtLimit()) {
                input.value = ''
                return false
            }
            
            selectedRecipients.push(trimmed)
            updateBadges()
            input.value = ''
            return true
        } finally {
            // Always release lock
            isAddingRecipient = false
        }
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
        // Check limit before adding
        if (isAtLimit()) {
            return false
        }
        const normalized = optionLookup.get(value) || value
        return addRecipient(normalized)
    }

    function tryAddRecipient(value) {
        const trimmed = value.trim()
        if (!trimmed) {
            return false
        }
        
        // Split on common separators (comma, semicolon, newline, tab)
        const recipients = trimmed.split(/[,;\n\r\t]+/).map(r => r.trim()).filter(r => r.length > 0)
        
        if (recipients.length === 0) {
            return false
        }
        
        // Check if we're already at the limit
        if (isAtLimit()) {
            return false
        }
        
        let added = false
        for (const recipient of recipients) {
            // CRITICAL: Check limit BEFORE each addition
            if (isAtLimit()) {
                break
            }
            
            // If it matches a datalist option, use the normalized username
            if (optionLookup.has(recipient)) {
                const normalized = optionLookup.get(recipient)
                // Verify limit again right before calling
                if (selectedRecipients.length < maxRecipients) {
                    if (addRecipient(normalized)) {
                        added = true
                    }
                    // Check again after addRecipient returns
                    if (isAtLimit()) {
                        break
                    }
                } else {
                    break
                }
            } else {
                // Otherwise, try to add as-is
                // Verify limit again right before calling
                if (selectedRecipients.length < maxRecipients) {
                    if (addRecipient(recipient)) {
                        added = true
                    }
                    // Check again after addRecipient returns
                    if (isAtLimit()) {
                        break
                    }
                } else {
                    break
                }
            }
        }
        
        return added
    }

    const form = container.closest('form')
    const submitButton = form?.querySelector('.share-submit')
    if (submitButton && !submitButton.dataset.shareSubmitInitialized) {
        submitButton.dataset.shareSubmitInitialized = 'true'
        submitButton.addEventListener('click', async event => {
            // Try to add any recipient from the input field
            tryAddRecipient(input.value.trim())
            
            // Ensure recipients are trimmed before submit check
            if (!hiddenInput.value || selectedRecipients.length === 0) {
                input.required = true
                input.reportValidity()
                event.preventDefault()
                event.stopPropagation()
                return
            }
            
            // Final check before submitting
            if (selectedRecipients.length > maxRecipients) {
                selectedRecipients = getSelectedRecipients().slice(0, maxRecipients)
                updateBadges()
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
        // Prevent input when disabled or at limit
        if (input.disabled || isAtLimit()) {
            e.preventDefault()
            e.stopPropagation()
            input.value = ''
            return
        }
        const value = e.target.value.trim()
        // Only check for exact matches on input - don't prevent datalist population
        // The datalist should populate to show suggestions as user types
        if (debounceTimer) {
            clearTimeout(debounceTimer)
        }
            debounceTimer = setTimeout(() => {
            if (!input.disabled && selectedRecipients.length < maxRecipients) {
                maybePopulateDatalist(value)
            }
        }, 200)
    })

    input.addEventListener('change', e => {
        // Don't process if input is disabled
        if (input.disabled || isAtLimit()) {
            return
        }
        const value = e.target.value.trim()
        if (!value) {
            return
        }
        if (!handleSelection(value)) {
            // If handleSelection didn't add it (not in datalist), try adding as raw value
            // but only if we're not at the limit
            if (value && selectedRecipients.length < maxRecipients) {
                addRecipient(value)
            } else {
            }
        }
    })

    input.addEventListener('blur', e => {
        // Don't process if input is disabled
        if (input.disabled || isAtLimit()) {
            return
        }
        const value = e.target.value.trim()
        if (!value) {
            return
        }
        if (!handleSelection(value)) {
            // If handleSelection didn't add it (not in datalist), try adding as raw value
            // but only if we're not at the limit
            if (value && selectedRecipients.length < maxRecipients) {
                addRecipient(value)
            } else {
            }
        }
    })

    input.addEventListener('keydown', e => {
        // Don't process Enter if input is disabled or at limit
        if (e.key === 'Enter') {
            e.preventDefault()
            if (input.disabled || isAtLimit()) {
                return
            }
            const value = input.value.trim()
            if (!handleSelection(value)) {
                // If handleSelection didn't add it (not in datalist), try adding as raw value
                // but only if we're not at the limit
                if (selectedRecipients.length < maxRecipients) {
                    addRecipient(value)
                } else {
                }
            }
        } else if (e.key === 'Backspace' && input.value === '' && selectedRecipients.length > 0) {
            const recipients = getSelectedRecipients()
            removeRecipient(recipients.length - 1)
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
