> **Reference doc, mostly about the game client.** This walks through wiring
> up *real* Firebase/Google sign-in end-to-end, including Unity-side steps.
> You do **not** need any of this for local backend development — the
> backend also accepts plain login/password auth with no Firebase project at
> all, see [`../SETUP.md#auth`](../SETUP.md#auth). Use this doc only if
> you're specifically standing up or debugging the Firebase login path.
> Some links below (`docs/feature/...`) point at files that live in the game
> client's repo, not this one.

---

# Firebase Auth Setup

Назначение: practical setup checklist для текущей реализации `Firebase auth flow`.

Канон фичи остаётся в:
- `docs/feature/auth_onboarding.md`
- `docs/feature/tasks_backlog.md`

Этот файл не заменяет feature-docs. Он описывает, что нужно сделать в Firebase Console и Unity, чтобы уже реализованный код заработал end-to-end.

## Что уже готово в коде

- Backend принимает `firebase_token` на `/auth/login` и `/auth/register`.
- Backend создаёт или находит игрока по `firebase_uid`.
- Unity `AuthScreen` умеет:
  - логиниться по Firebase ID token;
  - вызывать social bridge;
  - держать local dev fallback для editor/development build.
- В Unity добавлены bridge/source компоненты:
  - `FirebaseUnitySdkAuthBridge`
  - `GoogleSignInCredentialSource`
  - `DevelopmentFirebaseAuthBridge`
  - `DevelopmentSocialCredentialSource`

## Что нужно сделать тебе

### 1. Firebase Console

Нужно:
- создать или выбрать Firebase project;
- добавить Android app и, если нужен iOS, Apple app;
- включить `Authentication -> Sign-in method -> Google`;
- если нужен Apple sign-in, включить `Apple`;
- получить Web client ID для Google sign-in через Firebase/Google Cloud config;
- скачать `google-services.json` для Android.

Важно:
- для Google sign-in Unity flow нужен именно Web client ID, не только Android client ID;
- backend должен иметь валидный `firebase-credentials.json` для Firebase Admin SDK.

### 2. Backend

Нужно:
- положить service account credentials file;
- выставить env `FIREBASE_CREDENTIALS_FILE` на этот файл;
- перезапустить backend.

Проверка:
- backend не должен логировать `firebase auth is not configured`;
- после этого `/auth/login` с валидным Firebase ID token должен создавать session.

### 3. Unity packages / plugins

Нужно импортировать:
- Firebase Unity Auth SDK;
- Google Sign-In Unity plugin.

Код уже ожидает следующие runtime types:
- `Firebase.Auth.FirebaseAuth`
- `Firebase.Auth.GoogleAuthProvider`
- `Firebase.Auth.OAuthProvider`
- `Google.GoogleSignIn`
- `Google.GoogleSignInConfiguration`

Если этих типов нет, project всё равно компилируется, но social buttons не дадут реальный login.

### 4. Unity project files

Нужно:
- поместить `google-services.json` в проект по требованиям Firebase Unity setup;
- выполнить все post-import шаги Firebase External Dependency Manager, если пакет их добавит.

### 5. Scene wiring

В `SampleScene` уже есть объект с `AuthScreen`.

Нужно на тот же объект или рядом добавить и настроить:

Production / real Google path:
- `GoogleSignInCredentialSource`
- `FirebaseUnitySdkAuthBridge`

Связи:
- у `FirebaseUnitySdkAuthBridge.credentialSource` указать `GoogleSignInCredentialSource`;
- у `GoogleSignInCredentialSource.webClientId` вставить Web client ID;
- включить `enableGoogle` в `FirebaseUnitySdkAuthBridge`.

Development path:
- можно вместо этого использовать `DevelopmentFirebaseAuthBridge`;
- либо `DevelopmentSocialCredentialSource` + `FirebaseUnitySdkAuthBridge`.

### 6. Первичная проверка

Порядок:
1. Запустить backend с рабочим Firebase Admin credentials.
2. Открыть Unity scene.
3. Нажать Google login.
4. Убедиться, что Unity получает Firebase ID token.
5. Убедиться, что backend отвечает `session_token`.
6. Убедиться, что игрок попадает в session и дальше в onboarding/map flow.

## Что делать, если хочешь самый быстрый первый успех

Самый короткий путь:
- сначала подключить Firebase Unity Auth SDK;
- потом подключить Google Sign-In Unity plugin;
- потом настроить только Android/Google;
- только после этого возвращаться к Apple.

## Когда можно закрывать backlog пункт

`Firebase auth flow` можно отмечать завершённым, когда выполнено всё:
- работает реальный Google login в Unity;
- Unity получает Firebase ID token без ручного ввода;
- backend принимает token и создаёт session;
- новый игрок создаётся по `firebase_uid`;
- повторный вход возвращает существующего игрока;
- compile/tests остаются зелёными.
