@echo off
SETLOCAL EnableDelayedExpansion

REM Builds one exe per fake CPU, so the dialog can be looked at on machines
REM nobody here owns. Each of these is a normal build of the program with its
REM own hardcoded answer to "what processors does this machine have", taken
REM from the matching GetCpuInformation.<name>.go, so what comes out is the
REM real dialog laid out for that machine. They are for looking at, not for
REM shipping.
REM
REM Nothing here is built without the debug tag, and the debug tag on its own
REM builds nothing at all: it takes the real GetCpuInformation out, and every
REM fake one needs a vendor and a model named as well.

SET GOOS=windows
SET GOARCH=amd64
SET filename=GoInterruptPolicy
SET outdir=fakes

REM Vendor and model, as the build tags spell them, which is not always how the
REM file names spell them: GetCpuInformation.11400.go is built with 11400H, and
REM GetCpuInformation.5900x.go with 5900X. A new GetCpuInformation.*.go needs a
REM line here too, and the first line of that file says what to put.
SET models=Intel:11400H Intel:11400HPECORES Intel:13600KF Intel:13900 Intel:13900WithoutHT Intel:Fake8Threads Intel:FakeNumaCCD12Core AMD:5900X AMD:9950X3D AMD:Fake2CCD12CoreHT

IF NOT EXIST "%outdir%" MKDIR "%outdir%"

SET /A built=0
SET /A failed=0

FOR %%m IN (%models%) DO (
    FOR /F "tokens=1,2 delims=:" %%v IN ("%%m") DO (
        ECHO Building %%v %%w
        go build -tags "debug,%%v,%%w" -buildvcs=false -o "%outdir%\%filename%_%%w.exe"
        IF ERRORLEVEL 1 (
            SET /A failed+=1
            ECHO     FAILED
        ) ELSE (
            SET /A built+=1
        )
    )
)

ECHO.
IF !failed! EQU 0 (
    ECHO %outdir%\ : !built! built
) ELSE (
    ECHO %outdir%\ : !built! built, !failed! FAILED
)
ENDLOCAL
PAUSE
