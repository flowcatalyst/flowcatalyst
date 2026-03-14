<?php

namespace FlowCatalyst\Generated\Endpoint;

class GetApiMeClientsByClientIdApplication extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $clientId;
    /**
     * @param string $clientId
     */
    public function __construct(string $clientId)
    {
        $this->clientId = $clientId;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'GET';
    }
    public function getUri(): string
    {
        return str_replace(['{clientId}'], [$this->clientId], '/api/me/clients/{clientId}/applications');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\GetApiMeClientsByClientIdApplicationUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetApiMeClientsByClientIdApplicationForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\GetApiMeClientsByClientIdApplicationNotFoundException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse200
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse200', 'json');
        }
        if (is_null($contentType) === false && (401 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\GetApiMeClientsByClientIdApplicationUnauthorizedException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse401', 'json'), $response);
        }
        if (is_null($contentType) === false && (403 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\GetApiMeClientsByClientIdApplicationForbiddenException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse403', 'json'), $response);
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\GetApiMeClientsByClientIdApplicationNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse404', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return ['bearerAuth'];
    }
}