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
    const hiddenDateInput = dateControl.querySelector('.date')

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
      let composed = el.getElementsByClassName('date')[0]
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
