package rules

// mixedDefaultShellNames conservatively excludes shell-managed and shell-used
// parameters, their tied aliases, and module/widget parameters from the new
// mixed-default proof, even when an optional module is not loaded. It does not
// change the older numeric or literal-only exemptions.
// Sources are the released Zsh manual chapters:
// https://zsh.sourceforge.io/Doc/Release/Parameters.html
// https://zsh.sourceforge.io/Doc/Release/Zsh-Modules.html
// https://zsh.sourceforge.io/Doc/Release/Zsh-Line-Editor.html
// https://zsh.sourceforge.io/Doc/Release/Completion-Widgets.html
const mixedDefaultShellNames = " " +
	"ARGC ARGV0 BASH_REMATCH BAUD BUFFER BUFFERLINES CDPATH COLUMNS CONTEXT CORRECT_IGNORE " +
	"CORRECT_IGNORE_FILE CPUTYPE CURRENT CURSOR CUTBUFFER DIRSTACKSIZE EGID ENV EPOCHREALTIME " +
	"EPOCHSECONDS ERRNO EUID FCEDIT FIGNORE FPATH FUNCNEST GID HISTCHARS HISTCMD HISTFILE HISTNO " +
	"HISTORY_IGNORE HISTSIZE HOME HOST IFS IPREFIX ISEARCHMATCH_ACTIVE ISEARCHMATCH_END " +
	"ISEARCHMATCH_START ISUFFIX KEYBOARD_HACK KEYMAP KEYS KEYS_QUEUED_COUNT KEYTIMEOUT LANG " +
	"LASTABORTEDSEARCH LASTSEARCH LASTWIDGET LBUFFER LC_ALL LC_COLLATE LC_CTYPE LC_MESSAGES LC_NUMERIC " +
	"LC_TIME LINENO LINES LISTMAX LOGCHECK LOGNAME MACHTYPE MAIL MAILCHECK MAILPATH MANPATH MARK MATCH " +
	"MBEGIN MEND MODULE_PATH NULLCMD NUMERIC OLDPWD OPTARG OPTIND OSTYPE PATH PENDING POSTDISPLAY " +
	"POSTEDIT PPID PREBUFFER PREDISPLAY PREFIX PROMPT PROMPT2 PROMPT3 PROMPT4 PROMPT_EOL_MARK PS1 PS2 PS3 " +
	"PS4 PSVAR PWD QIPREFIX QISUFFIX RANDOM RBUFFER READNULLCMD REGION_ACTIVE REPLY REPORTMEMORY " +
	"REPORTTIME RPROMPT RPROMPT2 RPS1 RPS2 SAVEHIST SECONDS SHLVL SPROMPT STTY SUFFIX SUFFIX_ACTIVE " +
	"SUFFIX_END SUFFIX_START TERM TERMINFO TERMINFO_DIRS TIMEFMT TMOUT TMPPREFIX TMPSUFFIX " +
	"TRY_BLOCK_ERROR TRY_BLOCK_INTERRUPT TTY TTYIDLE UID UNDO_CHANGE_NO UNDO_LIMIT_NO USERNAME VENDOR " +
	"WATCH WATCHFMT WIDGET WIDGETFUNC WIDGETSTYLE WORDCHARS YANK_ACTIVE YANK_END YANK_START ZBEEP " +
	"ZCURSES_COLORS ZCURSES_COLOR_PAIRS ZDOTDIR ZFTP_ACCOUNT ZFTP_CODE ZFTP_COUNT ZFTP_FILE ZFTP_HOST " +
	"ZFTP_IP ZFTP_PORT ZFTP_PREFS ZFTP_PWD ZFTP_REPLY ZFTP_SESSION ZFTP_SIZE ZFTP_SYSTEM ZFTP_TMOUT " +
	"ZFTP_TRANSFER ZFTP_TYPE ZFTP_USER ZFTP_VERBOSE ZLE_LINE_ABORTED ZLE_RECURSIVE " +
	"ZLE_REMOVE_SUFFIX_CHARS ZLE_RPROMPT_INDENT ZLE_SPACE_SUFFIX_CHARS ZLE_STATE ZPCRE_OP ZSH_ARGZERO " +
	"ZSH_EVAL_CONTEXT ZSH_EXECUTION_STRING ZSH_NAME ZSH_PATCHLEVEL ZSH_SCRIPT ZSH_SUBSHELL ZSH_VERSION _ " +
	"aliases argv builtins cdpath commands compstate dirstack dis_aliases dis_builtins dis_functions " +
	"dis_functions_source dis_galiases dis_patchars dis_reswords dis_saliases epochtime errnos fignore " +
	"fpath funcfiletrace funcsourcetrace funcstack functions functions_source functrace galiases " +
	"histchars history historywords jobdirs jobstates jobtexts keymaps killring langinfo mailpath manpath " +
	"mapfile match mbegin mend module_path modules nameddirs options parameters patchars path pipestatus " +
	"prompt psvar region_highlight registers reply reswords saliases signals status sysparams termcap " +
	"terminfo userdirs usergroups watch widgets words zcurses_attrs zcurses_colors zcurses_keycodes " +
	"zcurses_windows zgdbm_tied zle_bracketed_paste zle_highlight zsh_eval_context zsh_scheduled_events "
