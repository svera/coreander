"use strict"

const searchFilters = document.getElementById('search-filters')
const searchFiltersForm = searchFilters.closest('form')

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
        if (parseInt(monthSelect.value) === 2) { // Only update if February is selected
            updateMaxDays(monthSelect, dayInput, yearInput, dateControl)
        }
        updateHiddenDateInput(dateControl)
    })

    // Update hidden date input when year input changes (for text input)
    yearInput.addEventListener('input', () => {
        updateHiddenDateInput(dateControl)
    })

    // Update hidden date input when day changes
    dayInput.addEventListener('change', () => {
        updateHiddenDateInput(dateControl)
    })

    dayInput.addEventListener('input', () => {
        updateHiddenDateInput(dateControl)
    })

    // Initial update of max days
    updateMaxDays(monthSelect, dayInput, yearInput, dateControl)
    // Initial update of hidden date input
    updateHiddenDateInput(dateControl)
})

searchFiltersForm.addEventListener('submit', event => {
  event.preventDefault()

  searchFiltersForm.querySelectorAll('.date-control').forEach(function (el) {
      if (el.getElementsByClassName('input-year')[0].value === '' || el.getElementsByClassName('input-year')[0].value === '0') return
      let composed = el.parentElement.querySelector('.date')
      if (!composed) return
      let year = el.getElementsByClassName('input-year')[0].value
      if (year.startsWith('-') || year.startsWith('+')) {
        year = year.substring(0, 1) + year.substring(1).padStart(4, '0')
      } else {
        year = year.padStart(4, '0')
      }
      composed.value = year + '-' + el.getElementsByClassName('input-month')[0].value + '-' + el.getElementsByClassName('input-day')[0].value.padStart(2, '0')
  })

  // Disable inputs with empty or zero values
  // This prevents empty inputs from being submitted
  searchFilters.querySelectorAll('input').forEach(input => {
    if (input.value === '' || input.value === '0') {
      input.setAttribute('disabled', 'disabled')
    }
  })

  searchFiltersForm.submit()
})

// Enable inputs when the page is shown
// This is useful for when the page is loaded from cache or the back button is used
window.addEventListener('pageshow', () => {
    searchFilters.querySelectorAll('input').forEach(input => {
      input.removeAttribute('disabled')
  })
})

// Load translations
let translations = {}
const i18nElement = document.getElementById('i18n')
if (i18nElement) {
    translations = JSON.parse(i18nElement.textContent).i18n
}

// Load subjects for autocomplete
const subjectsList = document.getElementById('subjects-list')
const subjectsInput = document.getElementById('subjects')
const subjectsHiddenInput = document.getElementById('subjects-hidden')
const subjectsBadgesContainer = document.getElementById('subjects-badges-container')

// Array to store selected subjects
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
            // Clear existing options
            subjectsList.innerHTML = ''
            // Add each subject as an option
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

// Update badges display
function updateSubjectBadges() {
    if (!subjectsBadgesContainer || !subjectsHiddenInput) return

    // Clear existing badges
    subjectsBadgesContainer.innerHTML = ''

    if (selectedSubjects.length === 0) {
        subjectsBadgesContainer.classList.add('d-none')
        subjectsHiddenInput.value = ''
        return
    }

    // Show container
    subjectsBadgesContainer.classList.remove('d-none')

    // Create badge for each selected subject
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

    // Update hidden input with comma-separated values
    subjectsHiddenInput.value = selectedSubjects.join(',')
}

// Add a subject
function addSubject(subject) {
    const trimmedSubject = subject.trim()
    if (!trimmedSubject) {
        return
    }

    // Check for duplicates (case-insensitive)
    const isDuplicate = selectedSubjects.some(existing =>
        existing.toLowerCase() === trimmedSubject.toLowerCase()
    )

    if (!isDuplicate) {
        selectedSubjects.push(trimmedSubject)
        updateSubjectBadges()
    }
    // Clear input even if duplicate to provide feedback
    subjectsInput.value = ''
}

// Remove a subject
function removeSubject(index) {
    selectedSubjects.splice(index, 1)
    updateSubjectBadges()
    subjectsInput.focus()
}

// Check if a value matches a datalist option
function matchesDatalistOption(value) {
    const options = Array.from(subjectsList.options)
    return options.some(option => option.value === value)
}

// Handle when a value might match a datalist option
function handlePotentialDatalistMatch(value) {
    if (!value) {
        return
    }
    if (matchesDatalistOption(value)) {
        addSubject(value)
    }
}

// Initialize on page load
if (subjectsInput && subjectsHiddenInput) {
    // Load initial subjects from hidden input
    const initialValue = subjectsHiddenInput.value
    if (initialValue) {
        const subjects = initialValue.split(',').map(s => s.trim()).filter(s => s)
        // Remove duplicates (case-insensitive) using a Set
        const seen = new Set()
        selectedSubjects = subjects.filter(subject => {
            const lower = subject.toLowerCase()
            if (seen.has(lower)) {
                return false
            }
            seen.add(lower)
            return true
        })
    }

    // Wait for DOM to be ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', updateSubjectBadges)
    } else {
        updateSubjectBadges()
    }

    // Track the last input value to detect when a datalist option is selected
    let lastInputValue = ''

    // Handle input changes - check if value matches a datalist option
    subjectsInput.addEventListener('input', (e) => {
        const value = e.target.value.trim()
        lastInputValue = value
        handlePotentialDatalistMatch(value)
    })

    // Handle change event (when autocomplete is used or datalist option is selected)
    subjectsInput.addEventListener('change', (e) => {
        const value = e.target.value.trim()
        if (value && value !== lastInputValue) {
            addSubject(value)
        }
    })

    // Handle blur event as fallback for datalist selection
    subjectsInput.addEventListener('blur', (e) => {
        handlePotentialDatalistMatch(e.target.value.trim())
    })

    // Handle Enter key to add subject
    subjectsInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
            e.preventDefault()
            const value = subjectsInput.value.trim()
            if (value) {
                addSubject(value)
            }
        } else if (e.key === 'Backspace' && subjectsInput.value === '' && selectedSubjects.length > 0) {
            // Remove last subject when backspace is pressed on empty input
            removeSubject(selectedSubjects.length - 1)
        }
    })
}
