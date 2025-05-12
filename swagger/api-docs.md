# Iznik API Documentation

This document provides an overview of the Iznik API, which is used for accessing functionality for freegling (free reuse) groups.

## Getting Started

The API is accessible via:

```
https://api.ilovefreegle.org/api/
```

For newer client applications, you can also use:

```
https://api.ilovefreegle.org/apiv2/
```

## Authentication

Most endpoints that require authentication use JWT (JSON Web Tokens). Include the token in the `Authorization` header:

```
Authorization: Bearer <your_token>
```

## API Categories

The API is organized into several categories:

- **Address**: Manage user addresses
- **Authority**: Access authority-specific messages
- **Chat**: Manage chat rooms and messages
- **Community Events**: Access community event information
- **Groups**: Access group information and messages
- **Isochrone**: Get travel distance visualizations
- **Jobs**: Access job listings
- **Locations**: Manage and search locations
- **Messages**: Core functionality for posting and viewing messages
- **Newsfeed**: Access the newsfeed
- **Notifications**: Manage user notifications
- **Stories**: Access user stories
- **User**: Manage user information
- **Volunteering**: Access volunteering opportunities

## Examples

### Listing Groups

```http
GET /api/group
```

### Getting a Specific Message

```http
GET /api/message/123
```

### Searching Messages

```http
GET /api/message/search/bicycle?groupids=456,789
```

## Rate Limiting

The API has rate limits to prevent abuse. Please ensure your applications make efficient use of the API by:

1. Caching results where appropriate
2. Batching requests where possible
3. Implementing exponential backoff for retries

## Support

If you need help with the API, please contact:

- Email: geeks@ilovefreegle.org
- Support: https://www.ilovefreegle.org/help