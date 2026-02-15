<?php

namespace FlowCatalyst\Generated\Endpoint;

class UpdateClientApplications extends \FlowCatalyst\Generated\Runtime\Client\BaseEndpoint implements \FlowCatalyst\Generated\Runtime\Client\Endpoint
{
    protected $id;
    /**
     * Sets which applications are enabled for this client
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdateClientApplicationsRequest $requestBody
     */
    public function __construct(string $id, ?\FlowCatalyst\Generated\Model\UpdateClientApplicationsRequest $requestBody = null)
    {
        $this->id = $id;
        $this->body = $requestBody;
    }
    use \FlowCatalyst\Generated\Runtime\Client\EndpointTrait;
    public function getMethod(): string
    {
        return 'PUT';
    }
    public function getUri(): string
    {
        return str_replace(['{id}'], [$this->id], '/api/admin/clients/{id}/applications');
    }
    public function getBody(\Symfony\Component\Serializer\SerializerInterface $serializer, $streamFactory = null): array
    {
        if ($this->body instanceof \FlowCatalyst\Generated\Model\UpdateClientApplicationsRequest) {
            return [['Content-Type' => ['application/json']], $serializer->serialize($this->body, 'json')];
        }
        return [[], null];
    }
    public function getExtraHeaders(): array
    {
        return ['Accept' => ['application/json']];
    }
    /**
     * {@inheritdoc}
     *
     * @throws \FlowCatalyst\Generated\Exception\UpdateClientApplicationsBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\UpdateClientApplicationsNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\UpdateClientApplicationsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\UpdateClientApplicationsForbiddenException
     *
     * @return null|\FlowCatalyst\Generated\Model\ApplicationsUpdatedResponse
     */
    protected function transformResponseBody(\Psr\Http\Message\ResponseInterface $response, \Symfony\Component\Serializer\SerializerInterface $serializer, ?string $contentType = null)
    {
        $status = $response->getStatusCode();
        $body = (string) $response->getBody();
        if (is_null($contentType) === false && (200 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            return $serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ApplicationsUpdatedResponse', 'json');
        }
        if (is_null($contentType) === false && (400 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\UpdateClientApplicationsBadRequestException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ErrorResponse', 'json'), $response);
        }
        if (is_null($contentType) === false && (404 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\UpdateClientApplicationsNotFoundException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\NotFoundResponse', 'json'), $response);
        }
        if (is_null($contentType) === false && (401 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\UpdateClientApplicationsUnauthorizedException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\UnauthorizedResponse', 'json'), $response);
        }
        if (is_null($contentType) === false && (403 === $status && mb_strpos(strtolower($contentType), 'application/json') !== false)) {
            throw new \FlowCatalyst\Generated\Exception\UpdateClientApplicationsForbiddenException($serializer->deserialize($body, 'FlowCatalyst\Generated\Model\ForbiddenResponse', 'json'), $response);
        }
    }
    public function getAuthenticationScopes(): array
    {
        return [];
    }
}