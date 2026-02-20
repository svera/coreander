"use strict"

// Load translations (shared)
let translations = {}
const i18nElement = document.getElementById('i18n')
if (i18nElement) {
    translations = JSON.parse(i18nElement.textContent).i18n
}

/**
 * Determines if a given year is a leap year
 * @param {number} year - The year to check
 * @returns {boolean} - True if the year is a leap year, false otherwise
 */
function isLeapYear(year) {
    // A year is a leap year if:
    // 1. It's divisible by 4 AND
    // 2. It's either NOT divisible by 100 OR it's divisible by 400
    return (year % 4 === 0) && (year % 100 !== 0 || year % 400 === 0)
}

/**
 * Updates the max attribute of the day input based on the selected month and year
 * @param {HTMLElement} monthSelect - The month select element
 * @param {HTMLElement} dayInput - The day input element
 * @param {HTMLElement} yearInput - The year input element
 * @param {HTMLElement} dateControl - The date-control container element (optional, for updating hidden input)
 */
function updateMaxDays(monthSelect, dayInput, yearInput, dateControl = null) {
    const month = parseInt(monthSelect.value)
    const year = parseInt(yearInput.value) || new Date().getFullYear()

    let maxDays = 31 // default max days

    switch (month) {
        case 2: // February
            maxDays = isLeapYear(year) ? 29 : 28
            break
        case 4: // April
        case 6: // June
        case 9: // September
        case 11: // November
            maxDays = 30
            break
    }

    // Update the max attribute
    dayInput.setAttribute('max', maxDays)

    // If current day value is greater than max days, set it to max days
    const currentDay = parseInt(dayInput.value)
    if (currentDay > maxDays) {
        dayInput.value = maxDays
        // Update hidden input if dateControl is provided
        if (dateControl) {
            updateHiddenDateInput(dateControl)
        }
    }
}

/**
 * Updates the hidden date input field with the composed date value
 * @param {HTMLElement} dateControl - The date-control container element
 */
function updateHiddenDateInput(dateControl) {
    const yearInput = dateControl.querySelector('.input-year')
    const monthSelect = dateControl.querySelector('.input-month')
    const dayInput = dateControl.querySelector('.input-day')
    const hiddenDateInput = dateControl.parentElement.querySelector('.date')

    // Only update if year has a value
    if (!yearInput.value || yearInput.value === '' || yearInput.value === '0') {
        hiddenDateInput.value = ''
        return
    }

    let year = yearInput.value
    if (year.startsWith('-') || year.startsWith('+')) {
        year = year.substring(0, 1) + year.substring(1).padStart(4, '0')
    } else {
        year = year.padStart(4, '0')
    }

    const month = monthSelect.value || '01'
    const day = (dayInput.value || '1').padStart(2, '0')

    hiddenDateInput.value = year + '-' + month + '-' + day
}

/**
 * Initialize search filters for a single container (main or sidebar).
 * @param {HTMLElement} searchFilters - The container element (#search-filters or #search-filters-sidebar)
 */
function initSearchFilters(searchFilters) {
    if (!searchFilters) return
    const searchFiltersForm = searchFilters.closest('form')
    if (!searchFiltersForm) return

    const idPrefix = searchFilters.id === 'search-filters-sidebar' ? 'sidebar-' : ''

    // Set up event listeners for all month selects
    searchFilters.querySelectorAll('.date-control').forEach(dateControl => {
        const monthSelect = dateControl.querySelector('.input-month')
        const dayInput = dateControl.querySelector('.input-day')
        const yearInput = dateControl.querySelector('.input-year')

        // Update max days when month changes
        monthSelect.addEventListener('change', () => {
            updateMaxDays(monthSelect, dayInput, yearInput, dateControl)
            updateHiddenDateInput(dateControl)
        })

        // Update max days when year changes (for February)
        yearInput.addEventListener('change', () => {
            if (parseInt(monthSelect.value) === 2) {
                updateMaxDays(monthSelect, dayInput, yearInput, dateControl)
            }
            updateHiddenDateInput(dateControl)
        })

        yearInput.addEventListener('input', () => {
            updateHiddenDateInput(dateControl)
        })

        dayInput.addEventListener('change', () => {
            updateHiddenDateInput(dateControl)
        })

        dayInput.addEventListener('input', () => {
            updateHiddenDateInput(dateControl)
        })

        updateMaxDays(monthSelect, dayInput, yearInput, dateControl)
        updateHiddenDateInput(dateControl)
    })

    function composeDateControls() {
        searchFiltersForm.querySelectorAll('.date-control').forEach(function (el) {
            const yearEl = el.querySelector('.input-year')
            if (!yearEl || (yearEl.value === '' || yearEl.value === '0')) return
            const composed = el.parentElement.querySelector('.date')
            if (!composed) return
            let year = yearEl.value
            if (year.startsWith('-') || year.startsWith('+')) {
                year = year.substring(0, 1) + year.substring(1).padStart(4, '0')
            } else {
                year = year.padStart(4, '0')
            }
            const month = el.querySelector('.input-month').value || '01'
            const day = (el.querySelector('.input-day').value || '1').padStart(2, '0')
            composed.value = year + '-' + month + '-' + day
        })
    }

    const isDocumentsPage = window.location.pathname === '/documents'

    let applyingFilters = false

    function applyFilters() {
        applyingFilters = true
        composeDateControls()
        const list = document.getElementById('list')
        const sidebarForm = document.getElementById('search-filters-form')
        if (list && sidebarForm && isDocumentsPage) {
            if (searchFiltersForm !== sidebarForm) {
                const formData = new FormData(searchFiltersForm)
                for (const [k, v] of formData.entries()) {
                    const el = sidebarForm.elements[k]
                    if (el) el.value = v
                }
                const sidebarContainer = document.getElementById('search-filters-sidebar')
                if (sidebarContainer) sidebarContainer.dispatchEvent(new CustomEvent('syncSubjectsFromHiddenInput'))
            }
            document.body.dispatchEvent(new CustomEvent('update'))
            const formData = new FormData(sidebarForm)
            const params = new URLSearchParams()
            for (const [k, v] of formData.entries()) {
                if (v != null && String(v).trim() !== '') params.append(k, v)
            }
            const queryString = params.toString()
            const url = '/documents' + (queryString ? '?' + queryString : '')
            history.replaceState(null, '', url)
            syncSidebarFormToOffcanvas()
        } else {
            const params = new URLSearchParams(new FormData(searchFiltersForm))
            window.location.href = '/documents?' + params.toString()
        }
        setTimeout(() => { applyingFilters = false }, 0)
    }

    let triggerSearchUpdate = null
    if (isDocumentsPage) {
        const FILTER_DEBOUNCE_MS = 600
        let applyFiltersDebounced
        function scheduleApplyFilters() {
            if (applyingFilters) return
            if (applyFiltersDebounced) clearTimeout(applyFiltersDebounced)
            applyFiltersDebounced = setTimeout(applyFilters, FILTER_DEBOUNCE_MS)
        }
        triggerSearchUpdate = scheduleApplyFilters

        searchFiltersForm.addEventListener('submit', (e) => {
            e.preventDefault()
            if (applyFiltersDebounced) clearTimeout(applyFiltersDebounced)
            applyFilters()
        })

        searchFiltersForm.addEventListener('input', () => scheduleApplyFilters())
    } else {
        searchFiltersForm.addEventListener('submit', () => {
            composeDateControls()
            searchFilters.querySelectorAll('input').forEach(input => {
                if (input.value === '' || input.value === '0') input.setAttribute('disabled', 'disabled')
            })
        })
    }

    // Subjects (scoped to this container)
    const subjectsList = document.getElementById(idPrefix + 'subjects-list')
    const subjectsInput = document.getElementById(idPrefix + 'subjects')
    const subjectsHiddenInput = document.getElementById(idPrefix + 'subjects-hidden')
    const subjectsBadgesContainer = document.getElementById(idPrefix + 'subjects-badges-container')
    let selectedSubjects = []

    if (subjectsList) {
        fetch('/subjects')
            .then(response => {
                if (!response.ok) {
                    throw new Error('Failed to fetch subjects')
                }
                return response.json()
            })
            .then(subjects => {
                subjectsList.innerHTML = ''
                subjects.forEach(subject => {
                    const option = document.createElement('option')
                    option.value = subject
                    subjectsList.appendChild(option)
                })
            })
            .catch(error => {
                console.error('Error loading subjects:', error)
            })
    }

    function updateSubjectBadges() {
        if (!subjectsBadgesContainer || !subjectsHiddenInput) return
        subjectsBadgesContainer.innerHTML = ''
        if (selectedSubjects.length === 0) {
            subjectsBadgesContainer.classList.add('d-none')
            subjectsHiddenInput.value = ''
            return
        }
        subjectsBadgesContainer.classList.remove('d-none')
        selectedSubjects.forEach((subject, index) => {
            const badge = document.createElement('span')
            badge.className = 'badge rounded-pill text-bg-primary d-inline-flex align-items-center'
            badge.style.pointerEvents = 'all'
            badge.textContent = subject
            const closeBtn = document.createElement('button')
            closeBtn.type = 'button'
            closeBtn.className = 'btn-close btn-close-white ms-1 mt-0 small'
            const removeSubjectLabel = translations.remove_subject ? translations.remove_subject.replace('%s', subject) : `Remove subject: ${subject}`
            closeBtn.setAttribute('aria-label', removeSubjectLabel)
            closeBtn.addEventListener('click', (e) => {
                e.preventDefault()
                e.stopPropagation()
                removeSubject(index)
            })
            badge.appendChild(closeBtn)
            subjectsBadgesContainer.appendChild(badge)
        })
        subjectsHiddenInput.value = selectedSubjects.join(',')
    }

    function addSubject(subject) {
        const trimmedSubject = subject.trim()
        if (!trimmedSubject) return
        const isDuplicate = selectedSubjects.some(existing =>
            existing.toLowerCase() === trimmedSubject.toLowerCase()
        )
        if (!isDuplicate) {
            selectedSubjects.push(trimmedSubject)
            updateSubjectBadges()
            if (triggerSearchUpdate) triggerSearchUpdate()
        }
        if (subjectsInput) subjectsInput.value = ''
    }

    function removeSubject(index) {
        selectedSubjects.splice(index, 1)
        updateSubjectBadges()
        if (triggerSearchUpdate) triggerSearchUpdate()
        if (subjectsInput) subjectsInput.focus()
    }

    function matchesDatalistOption(value) {
        if (!subjectsList) return false
        const options = Array.from(subjectsList.options)
        return options.some(option => option.value === value)
    }

    function handlePotentialDatalistMatch(value) {
        if (!value) return
        if (matchesDatalistOption(value)) {
            addSubject(value)
        }
    }

    if (subjectsInput && subjectsHiddenInput) {
        const initialValue = subjectsHiddenInput.value
        if (initialValue) {
            const subjects = initialValue.split(',').map(s => s.trim()).filter(s => s)
            const seen = new Set()
            selectedSubjects = subjects.filter(subject => {
                const lower = subject.toLowerCase()
                if (seen.has(lower)) return false
                seen.add(lower)
                return true
            })
        }
        if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', updateSubjectBadges)
        } else {
            updateSubjectBadges()
        }
        searchFilters.addEventListener('syncSubjectsFromHiddenInput', () => {
            if (!subjectsHiddenInput) return
            const value = subjectsHiddenInput.value
            const parsed = value ? value.split(',').map(s => s.trim()).filter(s => s) : []
            const seen = new Set()
            selectedSubjects.length = 0
            parsed.forEach(subject => {
                const lower = subject.toLowerCase()
                if (!seen.has(lower)) {
                    seen.add(lower)
                    selectedSubjects.push(subject)
                }
            })
            updateSubjectBadges()
        })
        let lastInputValue = ''
        subjectsInput.addEventListener('input', (e) => {
            const value = e.target.value.trim()
            lastInputValue = value
            handlePotentialDatalistMatch(value)
        })
        subjectsInput.addEventListener('change', (e) => {
            const value = e.target.value.trim()
            if (value && value !== lastInputValue) {
                addSubject(value)
            }
        })
        subjectsInput.addEventListener('blur', (e) => {
            handlePotentialDatalistMatch(e.target.value.trim())
        })
        subjectsInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault()
                const value = subjectsInput.value.trim()
                if (value) addSubject(value)
            } else if (e.key === 'Backspace' && subjectsInput.value === '' && selectedSubjects.length > 0) {
                removeSubject(selectedSubjects.length - 1)
            }
        })
    }
}

// Enable inputs when the page is shown
window.addEventListener('pageshow', () => {
    ['search-filters', 'search-filters-sidebar'].forEach(id => {
        const el = document.getElementById(id)
        if (el) {
            el.querySelectorAll('input').forEach(input => {
                input.removeAttribute('disabled')
            })
        }
    })
})

/**
 * Copy form values from source to target form (by field name).
 */
function copyFormValues(sourceForm, targetForm) {
    for (const el of sourceForm.elements) {
        if (!el.name) continue
        const target = targetForm.elements[el.name]
        if (target && target !== el) {
            if (target.type === 'checkbox' || target.type === 'radio') {
                target.checked = el.checked
            } else {
                target.value = el.value
            }
        }
    }
}

/**
 * Apply hidden date input values (YYYY-MM-DD) to the visible year/month/day inputs in each .date-control.
 */
function applyHiddenDatesToVisible(container) {
    if (!container) return
    container.querySelectorAll('.date-control').forEach(dateControl => {
        const hiddenInput = dateControl.parentElement.querySelector('input.date')
        if (!hiddenInput || !hiddenInput.value) return
        const parts = hiddenInput.value.split('-')
        if (parts.length < 3) return
        const yearInput = dateControl.querySelector('.input-year')
        const monthSelect = dateControl.querySelector('.input-month')
        const dayInput = dateControl.querySelector('.input-day')
        if (yearInput) yearInput.value = parts[0]
        if (monthSelect) monthSelect.value = parts[1]
        if (dayInput) dayInput.value = String(parseInt(parts[2], 10))
    })
}

/**
 * Sync sidebar filter form state to the offcanvas form and navbar searchbox.
 */
function syncSidebarFormToOffcanvas() {
    const sidebarForm = document.getElementById('search-filters-form')
    const offcanvasContainer = document.getElementById('search-filters')
    if (!sidebarForm) return
    const searchValue = sidebarForm.elements['search'] ? sidebarForm.elements['search'].value : ''
    const navSearchbox = document.getElementById('searchbox')
    if (navSearchbox) navSearchbox.value = searchValue
    if (!offcanvasContainer) return
    const offcanvasForm = offcanvasContainer.closest('form')
    if (!offcanvasForm) return
    copyFormValues(sidebarForm, offcanvasForm)
    const sidebarSubjectsHidden = document.getElementById('sidebar-subjects-hidden')
    const offcanvasSubjectsHidden = document.getElementById('subjects-hidden')
    if (sidebarSubjectsHidden && offcanvasSubjectsHidden) {
        offcanvasSubjectsHidden.value = sidebarSubjectsHidden.value
    }
    applyHiddenDatesToVisible(offcanvasContainer)
    offcanvasContainer.dispatchEvent(new CustomEvent('syncSubjectsFromHiddenInput'))
}

// Initialize all filter containers on the page
initSearchFilters(document.getElementById('search-filters'))
initSearchFilters(document.getElementById('search-filters-sidebar'))

// Keep sidebar and offcanvas filters in sync on the documents page
if (document.getElementById('search-filters-form') && document.getElementById('search-filters')) {
    const offcanvasEl = document.getElementById('search-filters-offcanvas')
    if (offcanvasEl) {
        offcanvasEl.addEventListener('shown.bs.offcanvas', () => syncSidebarFormToOffcanvas())
    }
}
