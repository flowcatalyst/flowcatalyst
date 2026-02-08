<?php

namespace FlowCatalyst\Generated\Endpoint;

class SdkGrantClientAccess extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $clientId;
    protected $id;
    /**
     * @param string $clientId
     * @param string $id
     */
    public function __construct(string $clientId, string $id)
    {
        $this->clientId = $clientId;
        $this->id = $id;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'POST';
    }
    public function getUri(): string
    {
        return str_replace(['{clientId}', '{id}'], [$this->clientId, $this->id], '/api/sdk/principals/{id}/clients/{clientId}');
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
     * @throws \FlowCatalyst\Generated\Exception\SdkGrantClientAccessBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\SdkGrantClientAccessNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkGrantClientAccessUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkGrantClientAccessForbiddenException
     *
     * @return null|\FlowCatalyst\Generated\Model\ClientAccessGrantDto
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (201 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ClientAccessGrantDto', 'json');
        }
        if (400 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkGrantClientAccessBadRequestException($response);
        }
        if (404 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkGrantClientAccessNotFoundException($response);
        }
        if (401 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkGrantClientAccessUnauthorizedException($response);
        }
        if (403 === $status) {
            throw new \FlowCatalyst\Generated\Exception\SdkGrantClientAccessForbiddenException($response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}