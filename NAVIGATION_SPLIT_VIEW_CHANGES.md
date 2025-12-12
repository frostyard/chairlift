# NavigationSplitView Migration

This document describes the changes made to convert the ChairLift application from using an AdwViewStack with AdwViewSwitcher to using an AdwNavigationSplitView.

## Changes Made

### 1. Updated window.ui
- Replaced the old structure with `AdwNavigationSplitView`
- Added a sidebar with a `GtkListBox` for navigation
- Added a content area with a `GtkStack` for displaying pages
- Wrapped everything in an `AdwToastOverlay` for toast notifications
- Increased default window size to 1000x700 for better split view experience

### 2. Updated window.py
- Changed template children to match new UI structure:
  - `split_view`: The main NavigationSplitView widget
  - `sidebar_list`: The ListBox containing navigation items
  - `content_stack`: The Stack containing content pages
- Changed `pages` from list to dictionary for easier page lookup
- Added `__on_sidebar_row_activated()` method to handle navigation
- Modified `__build_ui()` to:
  - Create navigation items dynamically
  - Populate the sidebar ListBox with ActionRows
  - Connect pages from ChairLiftUserHome to the content stack
  - Select the first item by default

### 3. Refactored user_home.py
- Removed GTK template decorator (no longer uses UI file)
- Changed from `Adw.Bin` to a regular class
- Changed from single component with ViewStack to a page manager
- Modified `__create_page()` to create individual ToolbarView pages with:
  - AdwHeaderBar
  - ScrolledWindow
  - AdwPreferencesPage
- Added `get_page()` method to retrieve pages by name
- Each page is now independent and wrapped in its own ToolbarView

### 4. Updated chairlift.gresource.xml
- Removed reference to `gtk/user-home.ui` as it's no longer used

### 5. Updated style.css
- Added styling for `.navigation-sidebar` class
- Added selected state styling for navigation items
- Improved visual feedback for navigation selection

## Architecture Changes

### Before
```
AdwApplicationWindow
  └─ AdwToolbarView
      └─ AdwViewStack (with ViewSwitcher in header)
          ├─ Applications Page
          ├─ Maintenance Page
          ├─ Updates Page
          ├─ System Page
          └─ Help Page
```

### After
```
AdwApplicationWindow
  └─ AdwToastOverlay
      └─ AdwNavigationSplitView
          ├─ Sidebar (AdwNavigationPage)
          │   └─ AdwToolbarView
          │       └─ GtkListBox (navigation items)
          └─ Content (AdwNavigationPage)
              └─ GtkStack
                  ├─ Applications Page (ToolbarView)
                  ├─ Maintenance Page (ToolbarView)
                  ├─ Updates Page (ToolbarView)
                  ├─ System Page (ToolbarView)
                  └─ Help Page (ToolbarView)
```

## Benefits

1. **Better Responsive Design**: NavigationSplitView automatically adapts to different window sizes, showing/hiding the sidebar as needed
2. **Improved Navigation**: Sidebar provides persistent navigation that's always visible on larger screens
3. **Modern GNOME HIG**: Follows current GNOME Human Interface Guidelines for settings-style applications
4. **Better Multi-tasking**: Users can see navigation context while viewing content
5. **Cleaner Architecture**: Each page is self-contained with its own toolbar

## Testing

To test the changes:
```bash
just build
./build/chairlift/chairlift
```

Or install locally:
```bash
just local
./install/bin/chairlift
```

## Notes

- The old `gtk/user-home.ui` file can be deleted as it's no longer used
- Navigation order is: Applications, Maintenance, Updates, System, Help
- Each page maintains its original functionality and preference groups
- The split view automatically handles narrow/wide window layouts
